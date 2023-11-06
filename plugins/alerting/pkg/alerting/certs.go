package alerting

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"
)

func (p *Plugin) loadCerts() *tls.Config {
	ctx, ca := context.WithTimeout(context.Background(), 10*time.Second)
	defer ca()
	gwConfig, err := p.gatewayConfig.GetContext(ctx)
	if err != nil {
		p.logger.Error(fmt.Sprintf("plugin startup failed : config was not loaded: %s", err))
		os.Exit(1)
	}
	alertingServerCa := gwConfig.Spec.Alerting.Certs.ServerCA
	alertingClientCa := gwConfig.Spec.Alerting.Certs.ClientCA
	alertingClientCert := gwConfig.Spec.Alerting.Certs.ClientCert
	alertingClientKey := gwConfig.Spec.Alerting.Certs.ClientKey

	p.logger.With(
		"alertingServerCa", alertingServerCa,
		"alertingClientCa", alertingClientCa,
		"alertingClientCert", alertingClientCert,
		"alertingClientKey", alertingClientKey,
	).Debug("loading certs")

	clientCert, err := tls.LoadX509KeyPair(alertingClientCert, alertingClientKey)
	if err != nil {
		p.logger.Error(fmt.Sprintf("failed to load alerting client key id : %s", err))
		os.Exit(1)
	}

	serverCaPool := x509.NewCertPool()
	serverCaData, err := os.ReadFile(alertingServerCa)
	if err != nil {
		p.logger.Error(fmt.Sprintf("failed to read alerting server CA %s", err))
		os.Exit(1)
	}

	if ok := serverCaPool.AppendCertsFromPEM(serverCaData); !ok {
		p.logger.Error(fmt.Sprintf("failed to load alerting server CA %s", err))
		os.Exit(1)
	}

	clientCaPool := x509.NewCertPool()
	clientCaData, err := os.ReadFile(alertingClientCa)
	if err != nil {
		p.logger.Error(fmt.Sprintf("failed to load alerting client CA : %s", err))
		os.Exit(1)
	}

	if ok := clientCaPool.AppendCertsFromPEM(clientCaData); !ok {
		p.logger.Error("failed to load alerting client Ca")
		os.Exit(1)
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{clientCert},
		ClientCAs:    clientCaPool,
		RootCAs:      serverCaPool,
	}
}
