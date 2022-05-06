package trust_test

import (
	"crypto/x509"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
)

func TestTrust(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Trust Suite")
}

func newTestCert() *x509.Certificate {
	certData := test.TestData("root_ca.crt")
	cert, err := util.ParsePEMEncodedCert(certData)
	Expect(err).NotTo(HaveOccurred())
	return cert
}
