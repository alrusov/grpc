package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/alrusov/config"
	"github.com/alrusov/log"
	"github.com/alrusov/misc"
	"github.com/alrusov/panic"
	"github.com/alrusov/stdhttp"

	proto "github.com/alrusov/grpc/test_proto"
)

//----------------------------------------------------------------------------------------------------------------------------//

const (
	flagWithClientConv = 0x0001
	flagWithServerConv = 0x0002
)

var (
	withNginx = false
	useSSL    = true // Если withNginx true, то useSSL будет принудительно true для клиента и false для сервера. См. код библиотеки и код ниже.
	srvAddr   = "127.0.0.1:9999"
)

type point struct {
	Measurement string  `json:"measurement"`
	Timestamp   int64   `json:"timestamp"`
	Name        string  `json:"position"`
	Val         float64 `json:"val"`
}

//----------------------------------------------------------------------------------------------------------------------------//

func TestGRPC(t *testing.T) {
	err := runTestGRPC(nil, 1000, 0)
	if err != nil {
		t.Fatal(err)
	}
}

//----------------------------------------------------------------------------------------------------------------------------//

func BenchmarkGRPCwithoutConv(b *testing.B) {
	benchmarkGRPC(b, 0)
}

func BenchmarkGRPCwithClientConv(b *testing.B) {
	benchmarkGRPC(b, flagWithClientConv)
}

func BenchmarkGRPCwithServerConv(b *testing.B) {
	benchmarkGRPC(b, flagWithServerConv)
}

func BenchmarkGRPCwithFullConv(b *testing.B) {
	benchmarkGRPC(b, flagWithClientConv|flagWithServerConv)
}

func benchmarkGRPC(b *testing.B, flags uint) {
	err := runTestGRPC(b, b.N, flags)
	if err != nil {
		b.Fatal(err)
	}
}

//----------------------------------------------------------------------------------------------------------------------------//

func runTestGRPC(b *testing.B, nPoints int, flags uint) (err error) {
	misc.Logger = log.StdLogger
	log.Disable()

	srvCfg := &Config{
		Addr:                srvAddr,
		UseSSL:              useSSL && !withNginx,
		SSLCombinedPem:      "$/server.pem",
		SkipTLSVerification: true,
	}

	sAddr := srvCfg.Addr

	clientCfg := &Config{
		Addr:                sAddr,
		UseSSL:              useSSL || withNginx,
		SkipTLSVerification: true,
	}

	msgs := misc.NewMessages()

	defer func() {
		_ = srvCfg.StopServer()

		if msgs.Len() > 0 {
			err = msgs.Error()
		}
	}()

	err = srvCfg.Check(nil)
	if err != nil {
		msgs.Add("srvCfg.Check: %s", err)
		return
	}

	err = clientCfg.Check(nil)
	if err != nil {
		msgs.Add("clientCfg.Check: %s", err)
		return
	}

	wg := new(sync.WaitGroup)
	wg.Add(1)

	go func() {
		panicID := panic.ID()
		defer panic.SaveStackToLogEx(panicID)

		done := false

		err := srvCfg.StartServer(
			func(grpcServer *grpc.Server) error {
				proto.RegisterHandlerServer(
					grpcServer,
					&testGRPC{flags: flags},
				)
				wg.Done()
				done = true
				return nil
			},
		)

		if err != nil && !done {
			msgs.AddError(err)
		}

		srvCfg.StopServer()

		if !done {
			wg.Done()
		}
	}()

	wg.Wait()
	misc.Sleep(500 * time.Millisecond)

	if !srvCfg.ServerStarted() {
		msgs.Add("server is not started")
		return
	}

	err = clientCfg.InitClient()
	if err != nil {
		return
	}

	defer clientCfg.CloseClient()

	client := proto.NewHandlerClient(clientCfg.GetClient())

	points := makePoints(nPoints)

	if b != nil && flags&flagWithClientConv != 0 {
		b.ResetTimer()
	}

	request := &proto.Points{
		Class:  "",
		Source: "source",
		Points: make([]*proto.Point, len(points)),
	}

	for i, p := range points {
		request.Points[i] = &proto.Point{
			Measurement: p.Measurement,
			Timestamp:   p.Timestamp,
			Name:        p.Name,
			Val:         p.Val,
		}
	}

	if b != nil && flags&flagWithClientConv == 0 {
		b.ResetTimer()
	}

	resp, err := client.Do(
		context.Background(),
		request,
		//grpc.MaxCallRecvMsgSize(int(cfg.MaxPacketSize)),
		//grpc.MaxCallSendMsgSize(int(cfg.MaxPacketSize)),
	)
	if err != nil {
		msgs.Add("client.Do: %s", err)
		return
	}

	if resp.Processed != uint32(len(request.Points)) {
		msgs.Add("got %d points, expected %d", resp.Processed, len(request.Points))
		return
	}

	return
}

type testGRPC struct {
	flags uint
	proto.UnimplementedHandlerServer
}

func (s *testGRPC) Do(ctx context.Context, request *proto.Points) (response *proto.PointsResponse, err error) {
	if s.flags&flagWithServerConv == 0 {
		response = &proto.PointsResponse{
			Processed: uint32(len(request.Points)),
			Error:     "",
		}
		return
	}

	list := make([]point, len(request.Points))
	for i, p := range request.Points {
		list[i] = point{
			Measurement: p.Measurement,
			Timestamp:   p.Timestamp,
			Name:        p.Name,
			Val:         p.Val,
		}
	}

	// чтобы задействовать points, а то оптимизатор может почикать кусок выше
	response = &proto.PointsResponse{
		Processed: uint32(len(list)),
		Error:     "",
	}

	return
}

//----------------------------------------------------------------------------------------------------------------------------//

// Для сравнения аналогичный бенчмарк для HTTP

func BenchmarkHTTPwithoutConv(b *testing.B) {
	benchmarkHTTP(b, 0)
}

func BenchmarkHTTPwithClientConv(b *testing.B) {
	benchmarkHTTP(b, flagWithClientConv)
}

func BenchmarkHTTPwithServerConv(b *testing.B) {
	benchmarkHTTP(b, flagWithServerConv)
}

func BenchmarkHTTPwithFullConv(b *testing.B) {
	benchmarkHTTP(b, flagWithClientConv|flagWithServerConv)
}

func benchmarkHTTP(b *testing.B, flags uint32) {
	misc.Logger = log.StdLogger
	log.Disable()

	url := fmt.Sprintf("http://%s/write", srvAddr)
	timeout := 5 * time.Second

	var listener *stdhttp.HTTP

	msgs := misc.NewMessages()
	defer func() {
		if listener != nil {
			listener.Close()
		}

		if msgs.Len() > 0 {
			b.Fatal(msgs.String())
		}
	}()

	config.SetCommon(&config.Common{})

	listener, err := stdhttp.NewListener(
		&config.Listener{
			Addr:    srvAddr,
			Timeout: config.Duration(timeout),
		},
		&testHTTP{
			flags: flags,
		},
	)
	if err != nil {
		msgs.Add("failed to create listener: %s", err)
		return
	}

	wg := new(sync.WaitGroup)
	wg.Add(1)

	go func() {
		panicID := panic.ID()
		defer panic.SaveStackToLogEx(panicID)

		wg.Done()

		err := listener.Start()
		if err != nil {
			msgs.Add("failed to create listener: %s", err)
			return
		}
	}()

	wg.Wait()

	points := makePoints(b.N)

	if flags&flagWithClientConv != 0 {
		b.ResetTimer()
	}

	buf, err := json.Marshal(points)
	if err != nil {
		msgs.Add("json.Marshal: %s", err)
		return
	}

	if flags&flagWithClientConv == 0 {
		b.ResetTimer()
	}

	opts := misc.StringMap{}
	headers := misc.StringMap{}

	bb, rr, err := stdhttp.Request(stdhttp.MethodPOST, url, timeout, opts, headers, buf)
	if err != nil {
		var data []byte
		if bb != nil {
			data = bb.Bytes()
		}
		msgs.Add(`failed to request: %s ("%s", %#v)`, err, data, rr)
	}
}

type testHTTP struct {
	flags uint32
}

func (h *testHTTP) Handler(id uint64, prefix string, path string, w http.ResponseWriter, r *http.Request) (processed bool) {
	switch path {
	case "/write":
		processed = true
		if h.flags&flagWithServerConv != 0 {
			body, code, err := stdhttp.ReadData(r.Header, r.Body)
			if err != nil {
				stdhttp.Error(id, false, w, r, code, "Error reading request body", err)
				return
			}

			var v []point
			err = json.Unmarshal(body.Bytes(), &v)
			if err != nil {
				stdhttp.Error(id, false, w, r, code, "json.Unmarshal", err)
				return
			}
		}

		stdhttp.WriteReply(w, r, http.StatusOK, stdhttp.ContentTypeText, nil, []byte("OK"))
		return
	}

	return
}

//----------------------------------------------------------------------------------------------------------------------------//

func makePoints(count int) (list []point) {
	now := misc.NowUnixNano()

	list = make([]point, count)
	for i := 0; i < count; i++ {
		list[i] = point{
			Measurement: "measurement",
			Timestamp:   now,
			Name:        fmt.Sprintf("Tag_%d", i),
			Val:         math.Sin(float64(i) * math.Pi / 180.),
		}
	}

	return
}

//----------------------------------------------------------------------------------------------------------------------------//
