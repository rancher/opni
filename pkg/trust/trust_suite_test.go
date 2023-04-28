package trust_test

import (
	"crypto/x509"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/test/testdata"
	"github.com/rancher/opni/pkg/util"
)

func TestTrust(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Trust Suite")
}

func newTestCert() *x509.Certificate {
	certData := testdata.TestData("root_ca.crt")
	cert, err := util.ParsePEMEncodedCert(certData)
	Expect(err).NotTo(HaveOccurred())
	return cert
}
