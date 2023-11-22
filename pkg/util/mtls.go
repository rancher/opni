package util

import (
	"crypto/tls"
	"crypto/x509"
	"os"
)

type MTLSSpecShape = struct {
	// Path to the server CA certificate.
	ServerCA string `json:"serverCA,omitempty"`
	// Path to the client CA certificate (not needed in all cases).
	ClientCA string `json:"clientCA,omitempty"`
	// Path to the certificate used for client-cert auth.
	ClientCert string `json:"clientCert,omitempty"`
	// Path to the private key used for client-cert auth.
	ClientKey string `json:"clientKey,omitempty"`
}

func LoadClientMTLSConfig(certs MTLSSpecShape) (*tls.Config, error) {
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
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{clientCert},
		ClientCAs:    clientCAPool,
		RootCAs:      serverCAPool,
	}, nil
}
