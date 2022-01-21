package util

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
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
