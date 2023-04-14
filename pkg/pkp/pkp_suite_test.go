package pkp_test

import (
	"crypto/x509"
	"encoding/json"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/test/testdata"
	"github.com/rancher/opni/pkg/util"
)

func TestUtil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Public Key Pinning Suite")
}

var (
	testFingerprints struct {
		TestData []struct {
			Cert         string             `json:"cert"`
			Fingerprints map[pkp.Alg]string `json:"fingerprints"`
		} `json:"testData"`
	}
	fullChain []*x509.Certificate
)

var _ = BeforeSuite(func() {
	Expect(json.Unmarshal(testdata.TestData("fingerprints.json"), &testFingerprints)).To(Succeed())
	Expect(testFingerprints.TestData).To(HaveLen(5))
	var err error
	fullChain, err = util.ParsePEMEncodedCertChain(testdata.TestData("full_chain.crt"))
	Expect(err).NotTo(HaveOccurred())
	Expect(fullChain).To(HaveLen(5))
})
