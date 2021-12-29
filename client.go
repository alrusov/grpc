package grpc

import (
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

//----------------------------------------------------------------------------------------------------------------------------//

func (cfg *Config) InitClient() (err error) {
	cfg.mutex.Lock()
	defer cfg.mutex.Unlock()

	if cfg.Addr == "" {
		err = fmt.Errorf("client address is undefined")
		return
	}

	if cfg.server != nil {
		err = fmt.Errorf("already used as a server")
		return
	}

	if cfg.client != nil {
		err = fmt.Errorf("client already initialized")
		return
	}

	var secureOpt grpc.DialOption
	if cfg.UseSSL {
		creds, e := cfg.makeCredentials()
		if e != nil {
			err = fmt.Errorf("makeCredentials: %s", e)
			return
		}
		secureOpt = grpc.WithTransportCredentials(creds)
	} else {
		secureOpt = grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	opts := []grpc.DialOption{
		secureOpt,
	}

	cfg.client, err = grpc.Dial(cfg.Addr, opts...)
	if err != nil {
		err = fmt.Errorf("grpc.Dial: %s", err)
		return
	}

	return
}

//----------------------------------------------------------------------------------------------------------------------------//

func (cfg *Config) GetClient() *grpc.ClientConn {
	cfg.mutex.Lock()
	defer cfg.mutex.Unlock()

	return cfg.client
}

//----------------------------------------------------------------------------------------------------------------------------//

func (cfg *Config) CloseClient() (err error) {
	cfg.mutex.Lock()
	defer cfg.mutex.Unlock()

	if cfg.client != nil {
		err = cfg.client.Close()
		cfg.client = nil
	}

	return
}

//----------------------------------------------------------------------------------------------------------------------------//
