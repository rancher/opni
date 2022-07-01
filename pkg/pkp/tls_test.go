package pkp_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gonum.org/v1/gonum/stat/combin"

	"github.com/rancher/opni/pkg/pkp"
)

var _ = Describe("TLS Config", Label("unit"), func() {
	When("creating a tls config with no pins", func() {
		It("should error", func() {
			conf, err := pkp.TLSConfig([]*pkp.PublicKeyPin{})
			Expect(conf).To(BeNil())
			Expect(err).To(MatchError(pkp.ErrNoPins))
		})
	})
	When("creating a tls config with invalid pins", func() {
		It("should return an error", func() {
			conf, err := pkp.TLSConfig([]*pkp.PublicKeyPin{
				{
					Algorithm:   pkp.Alg("sha257"),
					Fingerprint: make([]byte, 32),
				},
			})
			Expect(conf).To(BeNil())
			Expect(err).To(MatchError(pkp.ErrUnsupportedAlgorithm))
			conf, err = pkp.TLSConfig([]*pkp.PublicKeyPin{
				{
					Algorithm:   pkp.AlgSHA256,
					Fingerprint: []byte("sha256:AApyGp-JsUj8_5AapE2L3SqHR40IVe39jyay1Ul1Nn0"),
				},
			})
			Expect(conf).To(BeNil())
			Expect(err).To(MatchError(pkp.ErrMalformedPin))
		})
	})
	When("the peer's full cert chain is available", func() {
		It("should verify the connection when pinning any combination of certs", func() {
			for size := 1; size <= len(testFingerprints.TestData); size++ {
				for _, permutation := range combin.Permutations(len(testFingerprints.TestData), size) {
					pins := make([]*pkp.PublicKeyPin, size)
					for i, idx := range permutation {
						var err error
						pins[i], err = pkp.DecodePin(testFingerprints.TestData[idx].Fingerprints[pkp.AlgSHA256])
						Expect(err).NotTo(HaveOccurred())
					}
					cs := tls.ConnectionState{
						PeerCertificates: fullChain,
					}
					tlsConfig, err := pkp.TLSConfig(pins)
					Expect(err).NotTo(HaveOccurred())
					Expect(tlsConfig.VerifyConnection(cs)).To(Succeed())
				}
			}
		})
	})
	When("the peer's cert chain omits one or more certificates", func() {
		It("should verify the connection if any of the included certs are matched", func() {
			successes := 0
			failures := 0
			for upperBound := len(fullChain); upperBound > 0; upperBound-- {
				peerCerts := fullChain[:upperBound]
				for numPins := 1; numPins <= len(testFingerprints.TestData); numPins++ {
					for _, permutation := range combin.Permutations(len(testFingerprints.TestData), numPins) {
						pins := make([]*pkp.PublicKeyPin, numPins)
						shouldVerify := false
						for i, idx := range permutation {
							if idx < upperBound {
								shouldVerify = true
							}
							var err error
							pins[i], err = pkp.DecodePin(testFingerprints.TestData[idx].Fingerprints[pkp.AlgSHA256])
							Expect(err).NotTo(HaveOccurred())
						}
						cs := tls.ConnectionState{
							PeerCertificates: peerCerts,
						}
						tlsConfig, err := pkp.TLSConfig(pins)
						Expect(err).NotTo(HaveOccurred())
						if shouldVerify {
							Expect(tlsConfig.VerifyConnection(cs)).To(
								Succeed(),
								fmt.Sprintf("peerCerts: [0:%d], pins: %v", upperBound, permutation),
							)
							successes++
						} else {
							Expect(tlsConfig.VerifyConnection(cs)).To(
								MatchError(pkp.ErrCertValidationFailed),
								fmt.Sprintf("peerCerts: [0:%d], pins: %v", upperBound, permutation),
							)
							failures++
						}
					}
				}
			}
			Expect(successes).To(BeNumerically(">", 0))
			Expect(failures).To(BeNumerically(">", 0))
		})
	})
	When("the peer's cert chain is invalid", func() {
		It("should never verify the connection", func() {
			badChain := []*x509.Certificate{
				fullChain[1],
				fullChain[0],
				fullChain[2],
				fullChain[3],
				fullChain[4],
			}
			for size := 1; size <= len(testFingerprints.TestData); size++ {
				for _, permutation := range combin.Permutations(len(testFingerprints.TestData), size) {
					pins := make([]*pkp.PublicKeyPin, size)
					for i, idx := range permutation {
						var err error
						pins[i], err = pkp.DecodePin(testFingerprints.TestData[idx].Fingerprints[pkp.AlgSHA256])
						Expect(err).NotTo(HaveOccurred())
					}
					cs := tls.ConnectionState{
						PeerCertificates: badChain,
					}
					tlsConfig, err := pkp.TLSConfig(pins)
					Expect(err).NotTo(HaveOccurred())
					Expect(tlsConfig.VerifyConnection(cs)).To(MatchError(pkp.ErrCertValidationFailed))
				}
			}
		})
	})
})
