package grpc

import (
	"fmt"
	"net"

	"google.golang.org/grpc"
)

//----------------------------------------------------------------------------------------------------------------------------//

func (cfg *Config) StartServer(registrator HandlerRegistrator) (err error) {
	err = func() (err error) {
		// Выполняется в залоченном состоянии

		cfg.mutex.Lock()
		defer cfg.mutex.Unlock()

		if cfg.Addr == "" {
			err = fmt.Errorf("server address is undefined")
			return
		}

		if cfg.client != nil {
			err = fmt.Errorf("already used as a client")
			return
		}

		if cfg.listener != nil {
			err = fmt.Errorf("server already started")
			return
		}

		creds, err := cfg.makeCredentials()
		if err != nil {
			err = fmt.Errorf("makeCredentials: %s", err)
			return
		}

		opts := []grpc.ServerOption{
			grpc.MaxRecvMsgSize(cfg.MaxPacketSize),
			grpc.MaxSendMsgSize(cfg.MaxPacketSize),
			grpc.Creds(creds),
		}

		cfg.server = grpc.NewServer(opts...)

		err = registrator(cfg.server)
		if err != nil {
			return
		}

		cfg.listener, err = net.Listen("tcp", cfg.Addr)
		if err != nil {
			cfg.listener = nil
			err = fmt.Errorf("net.Listen: %s", err)
			return
		}

		return
	}()

	// Разлочено

	if err != nil {
		return
	}

	err = cfg.server.Serve(cfg.listener)
	if err != nil {
		_ = cfg.stopServer()
		err = fmt.Errorf("serve: %s", err)
		return
	}

	return cfg.stopServer()
}

//----------------------------------------------------------------------------------------------------------------------------//

func (cfg *Config) StopServer() (err error) {
	cfg.mutex.Lock()
	defer cfg.mutex.Unlock()

	return cfg.stopServer()
}

func (cfg *Config) stopServer() (err error) {
	if cfg.listener != nil {
		err = cfg.listener.Close()
		cfg.listener = nil
	}

	return
}

//----------------------------------------------------------------------------------------------------------------------------//

func (cfg *Config) ServerStarted() bool {
	cfg.mutex.Lock()
	defer cfg.mutex.Unlock()

	return cfg.listener != nil
}

//----------------------------------------------------------------------------------------------------------------------------//
