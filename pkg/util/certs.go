package util

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
)

func ParsePEMEncodedCertChain(chain []byte) ([]*x509.Certificate, error) {
	certs := make([]*x509.Certificate, 0)
	for len(chain) > 0 {
		var block *pem.Block
		var rest []byte
		block, rest = pem.Decode(chain)
		if block == nil {
			return nil, errors.New("failed to decode PEM data")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
		chain = rest
	}
	return certs, nil
}

func ParsePEMEncodedCert(data []byte) (*x509.Certificate, error) {
	var block *pem.Block
	block, _ = pem.Decode(data)
	if block == nil {
		return nil, errors.New("failed to decode PEM data")
	}
	return x509.ParseCertificate(block.Bytes)
}

type CertsSpecShape = struct {
	// Path to a PEM encoded CA certificate file. Mutually exclusive with CACertData
	CACert *string `json:"caCert,omitempty"`
	// String containing PEM encoded CA certificate data. Mutually exclusive with CACert
	CACertData []byte `json:"caCertData,omitempty"`
	// Path to a PEM encoded server certificate file. Mutually exclusive with ServingCertData
	ServingCert *string `json:"servingCert,omitempty"`
	// String containing PEM encoded server certificate data. Mutually exclusive with ServingCert
	ServingCertData []byte `json:"servingCertData,omitempty"`
	// Path to a PEM encoded server key file. Mutually exclusive with ServingKeyData
	ServingKey *string `json:"servingKey,omitempty"`
	// String containing PEM encoded server key data. Mutually exclusive with ServingKey
	ServingKeyData []byte `json:"servingKeyData,omitempty"`
}

func LoadServingCertBundle(certsSpec CertsSpecShape) (*tls.Certificate, *x509.CertPool, error) {
	var caCertData, servingCertData, servingKeyData []byte
	switch {
	case certsSpec.CACert != nil:
		data, err := os.ReadFile(*certsSpec.CACert)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load CA cert: %w", err)
		}
		caCertData = data
	case certsSpec.CACertData != nil:
		caCertData = certsSpec.CACertData
	default:
		return nil, nil, errors.New("no CA cert configured")
	}
	switch {
	case certsSpec.ServingCert != nil:
		data, err := os.ReadFile(*certsSpec.ServingCert)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load serving cert: %w", err)
		}
		servingCertData = data
	case certsSpec.ServingCertData != nil:
		servingCertData = certsSpec.ServingCertData
	default:
		return nil, nil, errors.New("no serving cert configured")
	}
	switch {
	case certsSpec.ServingKey != nil:
		data, err := os.ReadFile(*certsSpec.ServingKey)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load serving key: %w", err)
		}
		servingKeyData = data
	case certsSpec.ServingKeyData != nil:
		servingKeyData = certsSpec.ServingKeyData
	default:
		return nil, nil, errors.New("no serving key configured")
	}

	rootCA, err := ParsePEMEncodedCert(caCertData)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA cert: %w", err)
	}
	servingCert, err := tls.X509KeyPair(servingCertData, servingKeyData)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load TLS certificate: %w", err)
	}
	servingRootData := servingCert.Certificate[len(servingCert.Certificate)-1]
	servingRoot, err := x509.ParseCertificate(servingRootData)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse serving root certificate: %w", err)
	}
	if !rootCA.Equal(servingRoot) {
		servingCert.Certificate = append(servingCert.Certificate, rootCA.Raw)
	}
	caPool := x509.NewCertPool()
	caPool.AddCert(rootCA)
	return &servingCert, caPool, nil
}
