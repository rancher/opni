package cortex

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"golang.org/x/sync/singleflight"
)

var (
	sf singleflight.Group
)

func (p *Plugin) getOrLoadCortexCerts() *tls.Config {
	v, err, _ := sf.Do("cortexCerts", func() (interface{}, error) {
		ctx, ca := context.WithTimeout(context.Background(), 10*time.Second)
		defer ca()
		config, err := p.config.GetContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("plugin startup failed: config was not loaded: %w", err)
		}
		cortexServerCA := config.Spec.Cortex.Certs.ServerCA
		cortexClientCA := config.Spec.Cortex.Certs.ClientCA
		cortexClientCert := config.Spec.Cortex.Certs.ClientCert
		cortexClientKey := config.Spec.Cortex.Certs.ClientKey

		clientCert, err := tls.LoadX509KeyPair(cortexClientCert, cortexClientKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load cortex client keypair: %w", err)
		}
		serverCAPool := x509.NewCertPool()
		serverCAData, err := os.ReadFile(cortexServerCA)
		if err != nil {
			return nil, fmt.Errorf("failed to read cortex server CA: %w", err)
		}
		if ok := serverCAPool.AppendCertsFromPEM(serverCAData); !ok {
			return nil, fmt.Errorf("failed to load cortex server CA")
		}
		clientCAPool := x509.NewCertPool()
		clientCAData, err := os.ReadFile(cortexClientCA)
		if err != nil {
			return nil, fmt.Errorf("failed to read cortex client CA: %w", err)
		}
		if ok := clientCAPool.AppendCertsFromPEM(clientCAData); !ok {
			return nil, fmt.Errorf("failed to load cortex client CA")
		}
		return &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{clientCert},
			ClientCAs:    clientCAPool,
			RootCAs:      serverCAPool,
		}, nil
	})
	if err != nil {
		p.logger.With("err", err).Error("fatal error")
		os.Exit(1)
	}
	return v.(*tls.Config)
}
