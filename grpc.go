package grpc

import (
	"crypto/tls"
	"net"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/alrusov/config"
	"github.com/alrusov/misc"
)

//----------------------------------------------------------------------------------------------------------------------------//

type (
	// Настройки GRPC
	// Универсальный конфиг для сервера и клиента.
	// В принципе, можно было бы одновременно использовать один конфиг и для того, и для другого при условии, что все работает на одном хосте (т.е. Addr совпадает).
	// Но это в рамках разрабатываемой системы пока не требуется.
	// Поэтому, чтобы избежать путаницы, сделал проверку на попытку одновременного использование в качестве клиента и сервера. При этом выдается ошибка.
	Config struct {
		mutex *sync.Mutex

		listener net.Listener
		server   *grpc.Server

		client *grpc.ClientConn

		Addr                string          `toml:"addr"`                  // Адрес
		UseSSL              bool            `toml:"use-ssl"`               // Использовать SSL?
		SSLCombinedPem      string          `toml:"ssl-combined-pem"`      // Файл с pem сертификатом (key+crt). Используется только при UseSSL=true
		SkipTLSVerification bool            `toml:"skip-tls-verification"` // Не производить проверку сертификата контрагента?
		MaxPacketSize       int             `toml:"max-packet-size"`       // Максимальный размер передаваемого пакета. 0 - значение по умолчанию
		Timeout             config.Duration `toml:"timeout"`               // Таймаут

	}

	HandlerRegistrator func(*grpc.Server) error
)

/*
По поводу UseSSL.

Если сервер сидит за nginx, то у клиента обязательно должно быть UseSSL=true, а у сервера UseSSL=false.
Это потому, что до сервера всю работу с ssl сделает nginx.
В принципе, можно настроить nginx на http2 без ssl и grpc будет работать, но в этом случае браузером на другие (обычные) location
этого сервера (server в конфиге nginx) не зайти, так как известные браузеры не поддерживают такую конфигурацию (http2 без ssl).

В nginx добавляем что-то типа

location /points.Handler/ {
        grpc_pass 127.0.0.1:35819;
}
*/

const (
	defaultMaxPacketSize = 256 * 1024 * 1024
)

//----------------------------------------------------------------------------------------------------------------------------//

// Проверка валидности Config
func (x *Config) Check(cfg any) (err error) {
	x.mutex = new(sync.Mutex)

	msgs := misc.NewMessages()

	x.Addr = strings.TrimSpace(x.Addr)

	if x.SSLCombinedPem != "" {
		x.SSLCombinedPem, err = misc.AbsPath(x.SSLCombinedPem)
		if err != nil {
			msgs.Add("grpc.ssl-combined-pem: %s", err)
		}
	}

	if x.MaxPacketSize <= 0 {
		x.MaxPacketSize = defaultMaxPacketSize
	}

	if x.Timeout <= 0 {
		x.Timeout = config.ListenerDefaultTimeout
	}

	return msgs.Error()
}

//----------------------------------------------------------------------------------------------------------------------------//

func (cfg *Config) makeCredentials() (credentials.TransportCredentials, error) {
	if !cfg.UseSSL {
		return nil, nil
	}

	certs := []tls.Certificate{}

	if cfg.SSLCombinedPem != "" {
		cert, err := tls.LoadX509KeyPair(cfg.SSLCombinedPem, cfg.SSLCombinedPem)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}

	clientAuth := tls.RequireAndVerifyClientCert
	if cfg.SkipTLSVerification {
		clientAuth = tls.NoClientCert
	}

	config := &tls.Config{
		Certificates:       certs,
		ClientAuth:         clientAuth,              // Для сервера - что делать с сертификатом клиента
		InsecureSkipVerify: cfg.SkipTLSVerification, // Для клиента - что делать с сертификатом сервера
	}

	return credentials.NewTLS(config), nil
}

//----------------------------------------------------------------------------------------------------------------------------//
