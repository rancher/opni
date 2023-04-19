package reconciler

import (
	"crypto/tls"
	"crypto/x509"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestResources(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Opensearch Suite")
}

type mockCertReader struct{}

func (m *mockCertReader) GetTransportRootCAs() (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	return pool, nil
}

func (m *mockCertReader) GetHTTPRootCAs() (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	return pool, nil
}

func (m *mockCertReader) GetClientCert(_ string) (tls.Certificate, error) {
	return tls.Certificate{}, nil
}

func (m *mockCertReader) GetAdminClientCert() (tls.Certificate, error) {
	return tls.Certificate{}, nil
}
