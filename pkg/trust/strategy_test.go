package trust_test

import (
	"crypto/x509"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/trust"
)

var _ = Describe("Strategy", Label("unit"), func() {
	When("configuring the PKP trust strategy", func() {
		It("should build the correct tls config", func() {
			pin := pkp.NewSha256(newTestCert())
			conf := trust.StrategyConfig{
				PKP: &trust.PKPConfig{
					Pins: trust.NewPinSource([]*pkp.PublicKeyPin{pin}),
				},
			}
			strategy, err := conf.Build()
			Expect(err).ToNot(HaveOccurred())
			tls, err := strategy.TLSConfig()
			Expect(err).ToNot(HaveOccurred())
			Expect(tls.InsecureSkipVerify).To(BeTrue())
			Expect(tls.VerifyConnection).NotTo(BeNil())
		})
	})
	When("configuring the CA Certs trust strategy", func() {
		It("should build the correct tls config", func() {
			cert := newTestCert()
			conf := trust.StrategyConfig{
				CACerts: &trust.CACertsConfig{
					CACerts: trust.NewCACertsSource([]*x509.Certificate{cert}),
				},
			}
			strategy, err := conf.Build()
			Expect(err).ToNot(HaveOccurred())
			tls, err := strategy.TLSConfig()
			Expect(err).ToNot(HaveOccurred())
			Expect(tls.InsecureSkipVerify).To(BeFalse())
			Expect(tls.VerifyConnection).To(BeNil())
			Expect(tls.RootCAs).NotTo(BeNil())
			Expect(tls.RootCAs.Subjects()).To(HaveLen(1))
			Expect(tls.RootCAs.Subjects()[0]).To(Equal(cert.RawSubject))
		})
		It("should use the system cert pool if no CA certs are provided", func() {
			conf := trust.StrategyConfig{
				CACerts: &trust.CACertsConfig{},
			}
			strategy, err := conf.Build()
			Expect(err).ToNot(HaveOccurred())
			tls, err := strategy.TLSConfig()
			Expect(err).ToNot(HaveOccurred())
			Expect(tls.InsecureSkipVerify).To(BeFalse())
			Expect(tls.VerifyConnection).To(BeNil())
			systemCerts, err := x509.SystemCertPool()
			Expect(err).ToNot(HaveOccurred())
			Expect(tls.RootCAs.Subjects()).To(Equal(systemCerts.Subjects()))
		})
	})
	When("configuring the insecure trust strategy", func() {
		It("should build the correct tls config", func() {
			conf := trust.StrategyConfig{
				Insecure: &trust.InsecureConfig{},
			}
			strategy, err := conf.Build()
			Expect(err).ToNot(HaveOccurred())
			tls, err := strategy.TLSConfig()
			Expect(err).ToNot(HaveOccurred())
			Expect(tls.InsecureSkipVerify).To(BeTrue())
			Expect(tls.VerifyConnection).To(BeNil())
		})
	})
	It("should correctly interface with keyrings containing pins", func() {
		pin := pkp.NewSha256(newTestCert())
		kr := keyring.New(keyring.NewPKPKey([]*pkp.PublicKeyPin{pin}))
		pinSource := trust.NewKeyringPinSource(kr)
		conf := trust.StrategyConfig{
			PKP: &trust.PKPConfig{
				Pins: pinSource,
			},
		}
		strategy, err := conf.Build()
		Expect(err).ToNot(HaveOccurred())
		tls, err := strategy.TLSConfig()
		Expect(err).ToNot(HaveOccurred())
		Expect(tls.InsecureSkipVerify).To(BeTrue())
		Expect(tls.VerifyConnection).NotTo(BeNil())
	})
	It("should correctly interface with keyrings containing certs", func() {
		cert := newTestCert()
		kr := keyring.New(keyring.NewCACertsKey([]*x509.Certificate{cert}))
		certSource := trust.NewKeyringCACertsSource(kr)
		conf := trust.StrategyConfig{
			CACerts: &trust.CACertsConfig{
				CACerts: certSource,
			},
		}
		strategy, err := conf.Build()
		Expect(err).ToNot(HaveOccurred())
		tls, err := strategy.TLSConfig()
		Expect(err).ToNot(HaveOccurred())
		Expect(tls.InsecureSkipVerify).To(BeFalse())
		Expect(tls.VerifyConnection).To(BeNil())
		Expect(tls.RootCAs).NotTo(BeNil())
		Expect(tls.RootCAs.Subjects()).To(Equal([][]byte{cert.RawSubject}))
	})

	It("should return the correct persistent key for the pkp strategy", func() {
		pin := pkp.NewSha256(newTestCert())
		key := keyring.NewPKPKey([]*pkp.PublicKeyPin{pin})
		kr := keyring.New(key)
		pinSource := trust.NewKeyringPinSource(kr)
		conf := trust.StrategyConfig{
			PKP: &trust.PKPConfig{
				Pins: pinSource,
			},
		}
		strategy, err := conf.Build()
		Expect(err).ToNot(HaveOccurred())
		persistentKey := strategy.PersistentKey()
		Expect(err).ToNot(HaveOccurred())
		Expect(persistentKey).To(Equal(key))
	})

	It("should return the correct persistent key for the ca certs strategy", func() {
		cert := newTestCert()
		key := keyring.NewCACertsKey([]*x509.Certificate{cert})
		kr := keyring.New(key)
		certSource := trust.NewKeyringCACertsSource(kr)
		conf := trust.StrategyConfig{
			CACerts: &trust.CACertsConfig{
				CACerts: certSource,
			},
		}
		strategy, err := conf.Build()
		Expect(err).ToNot(HaveOccurred())
		persistentKey := strategy.PersistentKey()
		Expect(err).ToNot(HaveOccurred())
		Expect(persistentKey).To(Equal(key))
	})

	It("should return an empty key for the insecure strategy", func() {
		conf := trust.StrategyConfig{
			Insecure: &trust.InsecureConfig{},
		}
		strategy, err := conf.Build()
		Expect(err).ToNot(HaveOccurred())
		persistentKey := strategy.PersistentKey()
		Expect(err).ToNot(HaveOccurred())
		Expect(persistentKey).To(BeNil())
	})

	Context("error handling", func() {
		It("should return an error if the pkp strategy is configured with a nil pin source", func() {
			conf := trust.StrategyConfig{
				PKP: &trust.PKPConfig{
					Pins: nil,
				},
			}
			_, err := conf.Build()
			Expect(err).To(HaveOccurred())
		})
		It("should not return an error if the ca certs strategy is configured with a nil cert source", func() {
			conf := trust.StrategyConfig{
				CACerts: &trust.CACertsConfig{
					CACerts: nil,
				},
			}
			_, err := conf.Build()
			Expect(err).NotTo(HaveOccurred())
		})
		It("should handle invalid keyrings", func() {
			kr := keyring.New()
			pinSource := trust.NewKeyringPinSource(kr)
			conf := trust.StrategyConfig{
				PKP: &trust.PKPConfig{
					Pins: pinSource,
				},
			}
			_, err := conf.Build()
			Expect(err).To(HaveOccurred())

			kr = keyring.New(
				keyring.NewPKPKey([]*pkp.PublicKeyPin{
					pkp.NewSha256(newTestCert()),
				}),
				keyring.NewPKPKey([]*pkp.PublicKeyPin{
					pkp.NewBlake2b256(newTestCert()),
				}),
			)
			pinSource = trust.NewKeyringPinSource(kr)
			conf = trust.StrategyConfig{
				PKP: &trust.PKPConfig{
					Pins: pinSource,
				},
			}
			_, err = conf.Build()
			Expect(err).To(HaveOccurred())

			kr = keyring.New()
			certSource := trust.NewKeyringCACertsSource(kr)
			conf = trust.StrategyConfig{
				CACerts: &trust.CACertsConfig{
					CACerts: certSource,
				},
			}
			_, err = conf.Build()
			Expect(err).To(HaveOccurred())

			kr = keyring.New(
				keyring.NewCACertsKey([]*x509.Certificate{newTestCert()}),
				keyring.NewCACertsKey([]*x509.Certificate{newTestCert()}),
			)
			certSource = trust.NewKeyringCACertsSource(kr)
			conf = trust.StrategyConfig{
				CACerts: &trust.CACertsConfig{
					CACerts: certSource,
				},
			}
			_, err = conf.Build()
			Expect(err).To(HaveOccurred())

			invalidCert := newTestCert()
			invalidCert.Raw = []byte("invalid")
			kr = keyring.New(
				keyring.NewCACertsKey([]*x509.Certificate{invalidCert}),
			)
			certSource = trust.NewKeyringCACertsSource(kr)
			conf = trust.StrategyConfig{
				CACerts: &trust.CACertsConfig{
					CACerts: certSource,
				},
			}
			_, err = conf.Build()
			Expect(err).To(HaveOccurred())
		})

		It("should handle an empty configuration", func() {
			emptyConf := trust.StrategyConfig{}
			_, err := emptyConf.Build()
			Expect(err).To(HaveOccurred())
		})
	})
})
