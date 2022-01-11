package pkp_test

import (
	"crypto/x509"
	"embed"
	_ "embed"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/kralicky/opni-monitoring/pkg/pkp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUtil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Public Key Pinning Suite")
}

//go:embed testdata
var testDataFS embed.FS

func testData(filename string) []byte {
	data, err := fs.ReadFile(testDataFS, filepath.Join("testdata", filename))
	if err != nil {
		panic(err)
	}
	return data
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
	Expect(json.Unmarshal(testData("fingerprints.json"), &testFingerprints)).To(Succeed())
	Expect(testFingerprints.TestData).To(HaveLen(5))
	var err error
	fullChain, err = pkp.ParsePEMEncodedCertChain(testData("full_chain.crt"))
	Expect(err).NotTo(HaveOccurred())
	Expect(fullChain).To(HaveLen(5))
})
