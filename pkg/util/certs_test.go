package util_test

import (
	_ "embed"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
)

var _ = Describe("Cert Utils", Label("unit"), func() {
	testFullChain := test.TestData("full_chain.crt")
	It("should load a full cert chain", func() {
		chain, err := util.ParsePEMEncodedCertChain(testFullChain)
		Expect(err).NotTo(HaveOccurred())
		Expect(len(chain)).To(Equal(5))

		Expect(chain[0].Issuer.CommonName).To(Equal("Example Intermediate CA 3"))
		Expect(chain[0].Subject.CommonName).To(Equal("example.com"))

		Expect(chain[1].Issuer.CommonName).To(Equal("Example Intermediate CA 2"))
		Expect(chain[1].Subject.CommonName).To(Equal("Example Intermediate CA 3"))

		Expect(chain[2].Issuer.CommonName).To(Equal("Example Intermediate CA 1"))
		Expect(chain[2].Subject.CommonName).To(Equal("Example Intermediate CA 2"))

		Expect(chain[3].Issuer.CommonName).To(Equal("Example Root CA"))
		Expect(chain[3].Subject.CommonName).To(Equal("Example Intermediate CA 1"))

		Expect(chain[4].Issuer.CommonName).To(Equal("Example Root CA"))
		Expect(chain[4].Subject.CommonName).To(Equal("Example Root CA"))
	})
	It("should load a single cert", func() {
		cert, err := util.ParsePEMEncodedCert(testFullChain)
		Expect(err).NotTo(HaveOccurred())

		Expect(cert.Issuer.CommonName).To(Equal("Example Intermediate CA 3"))
		Expect(cert.Subject.CommonName).To(Equal("example.com"))
	})
	When("attempting to parse malformed data", func() {
		It("should return an error", func() {
			_, err := util.ParsePEMEncodedCertChain([]byte("invalid data"))
			Expect(err).To(MatchError("failed to decode PEM data"))
			_, err = util.ParsePEMEncodedCert([]byte("invalid data"))
			Expect(err).To(MatchError("failed to decode PEM data"))
		})
	})
	When("the correctly encoded data contains an invalid certificate", func() {
		notACert := []byte(`-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VwBCIEIM6i0VYYKNegxVFfCMXXbIBjjhDhfC30JPtkAImgL1Xw
-----END PRIVATE KEY-----`)
		It("should return an error", func() {
			_, err := util.ParsePEMEncodedCertChain(notACert)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("x509: "))
			_, err = util.ParsePEMEncodedCert(notACert)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("x509: "))
		})
	})
	It("should load a serving cert bundle", func() {
		_, _, err := util.LoadServingCertBundle(v1beta1.CertsSpec{
			CACertData:      test.TestData("root_ca.crt"),
			ServingCertData: test.TestData("localhost.crt"),
			ServingKeyData:  test.TestData("localhost.key"),
		})
		Expect(err).NotTo(HaveOccurred())
		_, _, err = util.LoadServingCertBundle(v1beta1.CertsSpec{
			CACert:      lo.ToPtr("../test/testdata/root_ca.crt"),
			ServingCert: lo.ToPtr("../test/testdata/localhost.crt"),
			ServingKey:  lo.ToPtr("../test/testdata/localhost.key"),
		})
		Expect(err).NotTo(HaveOccurred())
	})
	It("should handle errors when loading serving cert bundles", func() {
		_, _, err := util.LoadServingCertBundle(v1beta1.CertsSpec{
			CACertData:      test.TestData("root_ca.crt"),
			ServingCertData: test.TestData("localhost.crt"),
		})
		Expect(err).To(HaveOccurred())
		_, _, err = util.LoadServingCertBundle(v1beta1.CertsSpec{
			CACertData: test.TestData("root_ca.crt"),
		})
		Expect(err).To(HaveOccurred())
		_, _, err = util.LoadServingCertBundle(v1beta1.CertsSpec{})
		Expect(err).To(HaveOccurred())

		_, _, err = util.LoadServingCertBundle(v1beta1.CertsSpec{
			CACert:      lo.ToPtr("/does/not/exist"),
			ServingCert: lo.ToPtr("../test/testdata/localhost.crt"),
			ServingKey:  lo.ToPtr("../test/testdata/localhost.key"),
		})
		Expect(err).To(HaveOccurred())
		_, _, err = util.LoadServingCertBundle(v1beta1.CertsSpec{
			CACert:      lo.ToPtr("../test/testdata/root_ca.crt"),
			ServingCert: lo.ToPtr("/does/not/exist"),
			ServingKey:  lo.ToPtr("../test/testdata/localhost.key"),
		})
		Expect(err).To(HaveOccurred())
		_, _, err = util.LoadServingCertBundle(v1beta1.CertsSpec{
			CACert:      lo.ToPtr("../test/testdata/root_ca.crt"),
			ServingCert: lo.ToPtr("../test/testdata/localhost.crt"),
			ServingKey:  lo.ToPtr("/does/not/exist"),
		})
		Expect(err).To(HaveOccurred())

		_, _, err = util.LoadServingCertBundle(v1beta1.CertsSpec{
			CACertData:  []byte("invalid"),
			ServingCert: lo.ToPtr("../test/testdata/localhost.crt"),
			ServingKey:  lo.ToPtr("../test/testdata/localhost.key"),
		})
		Expect(err).To(HaveOccurred())
		_, _, err = util.LoadServingCertBundle(v1beta1.CertsSpec{
			CACert:          lo.ToPtr("../test/testdata/root_ca.crt"),
			ServingCertData: []byte("invalid"),
			ServingKey:      lo.ToPtr("../test/testdata/localhost.key"),
		})
		Expect(err).To(HaveOccurred())
		_, _, err = util.LoadServingCertBundle(v1beta1.CertsSpec{
			CACert:         lo.ToPtr("../test/testdata/root_ca.crt"),
			ServingCert:    lo.ToPtr("../test/testdata/localhost.crt"),
			ServingKeyData: []byte("invalid"),
		})
		Expect(err).To(HaveOccurred())
	})
})
