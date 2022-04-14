package cortex

import (
	"crypto/tls"
	"crypto/x509"
	"os"
)

func (p *Plugin) loadCortexCerts() *tls.Config {
	lg := p.logger
	config := p.config.Get()

	cortexServerCA := config.Spec.Cortex.Certs.ServerCA
	cortexClientCA := config.Spec.Cortex.Certs.ClientCA
	cortexClientCert := config.Spec.Cortex.Certs.ClientCert
	cortexClientKey := config.Spec.Cortex.Certs.ClientKey

	clientCert, err := tls.LoadX509KeyPair(cortexClientCert, cortexClientKey)
	if err != nil {
		lg.With(
			"err", err,
		).Error("fatal: failed to load cortex client keypair")
		os.Exit(1)
	}
	serverCAPool := x509.NewCertPool()
	serverCAData, err := os.ReadFile(cortexServerCA)
	if err != nil {
		lg.With(
			"err", err,
		).Error("fatal: failed to read cortex server CA")
		os.Exit(1)
	}
	if ok := serverCAPool.AppendCertsFromPEM(serverCAData); !ok {
		lg.Error("fatal: failed to load cortex server CA")
		os.Exit(1)
	}
	clientCAPool := x509.NewCertPool()
	clientCAData, err := os.ReadFile(cortexClientCA)
	if err != nil {
		lg.With(
			"err", err,
		).Error("fatal: failed to read cortex client CA")
		os.Exit(1)
	}
	if ok := clientCAPool.AppendCertsFromPEM(clientCAData); !ok {
		lg.Error("fatal: failed to load cortex client CA")
		os.Exit(1)
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{clientCert},
		ClientCAs:    clientCAPool,
		RootCAs:      serverCAPool,
	}
}
