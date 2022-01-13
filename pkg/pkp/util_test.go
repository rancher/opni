package pkp_test

import (
	_ "embed"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kralicky/opni-monitoring/pkg/pkp"
	"github.com/kralicky/opni-monitoring/pkg/test"
)

var _ = Describe("PKP Utils", func() {
	testFullChain := test.TestData("full_chain.crt")
	It("should load a full cert chain", func() {
		chain, err := pkp.ParsePEMEncodedCertChain(testFullChain)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(chain)).To(Equal(5))

		Expect(chain[0].SerialNumber.Text(16)).To(Equal("dc4a034e650266aa85a7f70caef6112e"))
		Expect(chain[0].Issuer.CommonName).To(Equal("Example Intermediate CA 3"))
		Expect(chain[0].Subject.CommonName).To(Equal("example.com"))

		Expect(chain[1].SerialNumber.Text(16)).To(Equal("aa8de8c3ad6798e77715e96e86381317"))
		Expect(chain[1].Issuer.CommonName).To(Equal("Example Intermediate CA 2"))
		Expect(chain[1].Subject.CommonName).To(Equal("Example Intermediate CA 3"))

		Expect(chain[2].SerialNumber.Text(16)).To(Equal("13c41b72862a6f96aa2890ca7ad7cde3"))
		Expect(chain[2].Issuer.CommonName).To(Equal("Example Intermediate CA 1"))
		Expect(chain[2].Subject.CommonName).To(Equal("Example Intermediate CA 2"))

		Expect(chain[3].SerialNumber.Text(16)).To(Equal("652c0336d0a8002ec196bb508b3ca5de"))
		Expect(chain[3].Issuer.CommonName).To(Equal("Example Root CA"))
		Expect(chain[3].Subject.CommonName).To(Equal("Example Intermediate CA 1"))

		Expect(chain[4].SerialNumber.Text(16)).To(Equal("d90262e46f1c59ef13260c0dbc4d55a7"))
		Expect(chain[4].Issuer.CommonName).To(Equal("Example Root CA"))
		Expect(chain[4].Subject.CommonName).To(Equal("Example Root CA"))
	})
	It("should load a single cert", func() {
		cert, err := pkp.ParsePEMEncodedCert(testFullChain)
		Expect(err).NotTo(HaveOccurred())

		Expect(cert.SerialNumber.Text(16)).To(Equal("dc4a034e650266aa85a7f70caef6112e"))
		Expect(cert.Issuer.CommonName).To(Equal("Example Intermediate CA 3"))
		Expect(cert.Subject.CommonName).To(Equal("example.com"))
	})
	When("attempting to parse malformed data", func() {
		It("should return an error", func() {
			_, err := pkp.ParsePEMEncodedCertChain([]byte("invalid data"))
			Expect(err).To(MatchError("failed to decode PEM data"))
			_, err = pkp.ParsePEMEncodedCert([]byte("invalid data"))
			Expect(err).To(MatchError("failed to decode PEM data"))
		})
	})
	When("the correctly encoded data contains an invalid certificate", func() {
		notACert := []byte(`-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VwBCIEIM6i0VYYKNegxVFfCMXXbIBjjhDhfC30JPtkAImgL1Xw
-----END PRIVATE KEY-----`)
		It("should return an error", func() {
			_, err := pkp.ParsePEMEncodedCertChain(notACert)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("x509: "))
			_, err = pkp.ParsePEMEncodedCert(notACert)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("x509: "))
		})
	})
})
