package cortex

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/rancher/opni/pkg/config/v1beta1"
)

func LoadTLSConfig(config *v1beta1.GatewayConfig) (*tls.Config, error) {
	cortexServerCA := config.Spec.Cortex.Certs.ServerCA
	cortexClientCA := config.Spec.Cortex.Certs.ClientCA
	cortexClientCert := config.Spec.Cortex.Certs.ClientCert
	cortexClientKey := config.Spec.Cortex.Certs.ClientKey

	clientCert, err := tls.LoadX509KeyPair(cortexClientCert, cortexClientKey)
	if err != nil {
		return nil, err
	}
	serverCAPool := x509.NewCertPool()
	serverCAData, err := os.ReadFile(cortexServerCA)
	if err != nil {
		return nil, err
	}
	if ok := serverCAPool.AppendCertsFromPEM(serverCAData); !ok {
		return nil, err
	}
	clientCAPool := x509.NewCertPool()
	clientCAData, err := os.ReadFile(cortexClientCA)
	if err != nil {
		return nil, err
	}
	if ok := clientCAPool.AppendCertsFromPEM(clientCAData); !ok {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{clientCert},
		ClientCAs:    clientCAPool,
		RootCAs:      serverCAPool,
	}, nil
}
