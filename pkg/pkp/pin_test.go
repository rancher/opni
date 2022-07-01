package pkp_test

import (
	"crypto/x509"
	_ "embed"
	"encoding/base64"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
)

// Fingerprints computed using:
// [sha256] openssl x509 -in $f -pubkey | openssl ec -pubin -outform der 2>/dev/null | openssl dgst -binary | base64 | tr '/+' '_-' | tr -d '='
// [b2b256] openssl x509 -in $f -pubkey | openssl ec -pubin -outform der 2>/dev/null | b2sum -l 256 | xxd -r -p | base64 | tr '/+' '_-' | tr -d '='

var _ = Describe("Public Key Pinning", Label("unit"), func() {
	It("should correctly encode and decode pins", func() {
		bytes := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
		b64 := base64.RawURLEncoding.EncodeToString(bytes)
		p1 := &pkp.PublicKeyPin{
			Algorithm:   pkp.AlgSHA256,
			Fingerprint: bytes,
		}
		Expect(p1.Encode()).To(Equal("sha256:" + b64))
		p2 := &pkp.PublicKeyPin{
			Algorithm:   pkp.AlgB2B256,
			Fingerprint: bytes,
		}
		Expect(p2.Encode()).To(Equal("b2b256:" + b64))

		d1, err := pkp.DecodePin("sha256:" + b64)
		Expect(err).To(BeNil())
		Expect(d1.Equal(p1)).To(BeTrue())

		d2, err := pkp.DecodePin("b2b256:" + b64)
		Expect(err).To(BeNil())
		Expect(d2.Equal(p2)).To(BeTrue())
	})
	It("should compute correct certificate fingerprints", func() {
		Expect(testFingerprints.TestData).To(HaveLen(5))
		for _, actual := range testFingerprints.TestData {
			certData := test.TestData(actual.Cert)
			cert, err := util.ParsePEMEncodedCert(certData)
			Expect(err).NotTo(HaveOccurred())
			for _, alg := range []pkp.Alg{pkp.AlgSHA256, pkp.AlgB2B256} {
				computed, err := pkp.New(cert, alg)
				Expect(err).NotTo(HaveOccurred())
				encoded := computed.Encode()
				Expect(encoded).To(Equal(actual.Fingerprints[alg]))
				decodedPin, err := pkp.DecodePin(encoded)
				Expect(err).To(BeNil())
				Expect(decodedPin.Equal(computed)).To(BeTrue())
			}
		}
	})
	It("should correctly deep-copy", func() {
		certData := test.TestData("root_ca.crt")
		cert, err := util.ParsePEMEncodedCert(certData)
		Expect(err).NotTo(HaveOccurred())
		pin := pkp.NewSha256(cert)
		copied := pin.DeepCopy()
		Expect(pin.Equal(copied)).To(BeTrue())
		copied.Fingerprint = []byte{0x01, 0x02, 0x03}
		Expect(pin.Equal(copied)).To(BeFalse())
	})
	It("should handle errors", func() {
		By("checking for unsupported algorithms")
		_, err := pkp.New(&x509.Certificate{}, pkp.Alg("test"))
		Expect(err).To(MatchError(pkp.ErrUnsupportedAlgorithm))
		_, err = pkp.DecodePin("sha257:dGVzdAo")
		Expect(err).To(MatchError(pkp.ErrUnsupportedAlgorithm))
		By("checking for missing algorithms")
		_, err = pkp.DecodePin("test")
		Expect(err).To(MatchError(pkp.ErrMissingAlgorithm))
		By("checking for malformed pins")
		_, err = pkp.DecodePin("sha256:test:")
		Expect(err).To(MatchError(pkp.ErrMalformedPin))
		_, err = pkp.DecodePin("sha256:not~ba$e64")
		Expect(err).To(MatchError(pkp.ErrMalformedPin))
	})
})
