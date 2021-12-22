package util

import (
	"crypto/x509"
	"fmt"

	"golang.org/x/crypto/blake2b"
)

func CACertHash(cert *x509.Certificate) ([]byte, error) {
	if !cert.IsCA {
		return nil, fmt.Errorf("not a CA certificate")
	}
	sum := blake2b.Sum256(cert.RawSubjectPublicKeyInfo)
	return sum[:], nil
}
