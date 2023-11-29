package gateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/rancher/opni/pkg/logger"
)

func (p *Plugin) loadCortexCerts() *tls.Config {
	lg := logger.PluginLoggerFromContext(p.ctx)

	ctx, ca := context.WithTimeout(context.Background(), 10*time.Second)
	defer ca()
	config, err := p.config.GetContext(ctx)
	if err != nil {
		lg.Error(fmt.Sprintf("plugin startup failed: config was not loaded: %v", err))
		os.Exit(1)
	}
	cortexServerCA := config.Spec.Cortex.Certs.ServerCA
	cortexClientCA := config.Spec.Cortex.Certs.ClientCA
	cortexClientCert := config.Spec.Cortex.Certs.ClientCert
	cortexClientKey := config.Spec.Cortex.Certs.ClientKey

	clientCert, err := tls.LoadX509KeyPair(cortexClientCert, cortexClientKey)
	if err != nil {
		lg.Error(fmt.Sprintf("failed to load cortex client keypair: %v", err))
		os.Exit(1)
	}
	serverCAPool := x509.NewCertPool()
	serverCAData, err := os.ReadFile(cortexServerCA)
	if err != nil {
		lg.Error(fmt.Sprintf("failed to read cortex server CA: %v", err))
		os.Exit(1)
	}
	if ok := serverCAPool.AppendCertsFromPEM(serverCAData); !ok {
		lg.Error("failed to load cortex server CA")
		os.Exit(1)
	}
	clientCAPool := x509.NewCertPool()
	clientCAData, err := os.ReadFile(cortexClientCA)
	if err != nil {
		lg.Error(fmt.Sprintf("failed to read cortex client CA: %v", err))
		os.Exit(1)
	}
	if ok := clientCAPool.AppendCertsFromPEM(clientCAData); !ok {
		lg.Error("failed to load cortex client CA")
		os.Exit(1)
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{clientCert},
		ClientCAs:    clientCAPool,
		RootCAs:      serverCAPool,
	}
}
