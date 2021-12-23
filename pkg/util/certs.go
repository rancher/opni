package util

import (
	"crypto/x509"
	"encoding/pem"
	"errors"

	"golang.org/x/crypto/blake2b"
)

func CertSPKIHash(cert *x509.Certificate) []byte {
	sum := blake2b.Sum256(cert.RawSubjectPublicKeyInfo)
	return sum[:]
}

func ParsePEMEncodedCertChain(chain []byte) ([]*x509.Certificate, error) {
	certs := make([]*x509.Certificate, 0)
	for len(chain) > 0 {
		var block *pem.Block
		var rest []byte
		block, rest = pem.Decode(chain)
		if block == nil {
			return nil, errors.New("failed to parse certificate")
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
