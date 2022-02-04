package util

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
)

func LoadClientMTLSConfig(certs *v1beta1.MTLSSpec) (*tls.Config, error) {
	clientCert, err := tls.LoadX509KeyPair(certs.ClientCert, certs.ClientKey)
	if err != nil {
		return nil, err
	}

	clientCAPool := x509.NewCertPool()
	if certs.ClientCA != "" {
		clientCAData, err := os.ReadFile(certs.ClientCA)
		if err != nil {
			return nil, err
		}
		clientCA, err := ParsePEMEncodedCert(clientCAData)
		if err != nil {
			return nil, err
		}
		clientCAPool.AddCert(clientCA)
	}

	serverCAPool := x509.NewCertPool()
	if certs.ServerCA != "" {
		serverCAData, err := os.ReadFile(certs.ServerCA)
		if err != nil {
			return nil, err
		}
		serverCA, err := ParsePEMEncodedCert(serverCAData)
		if err != nil {
			return nil, err
		}
		serverCAPool.AddCert(serverCA)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		ClientCAs:    clientCAPool,
		RootCAs:      serverCAPool,
	}, nil
}
