package trust_test

import (
	"crypto/x509"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"

	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/trust"
)

var _ = Describe("Flags", Label("unit"), func() {
	It("should build the pkp strategy from flags", func() {
		pin := pkp.NewSha256(newTestCert())
		flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
		trust.BindFlags(flagset)
		Expect(flagset.Parse([]string{
			"--trust-strategy=pkp",
			"--pin=" + pin.Encode(),
		})).To(Succeed())

		conf, err := trust.BuildConfigFromFlags(flagset)
		Expect(err).NotTo(HaveOccurred())
		strategy, err := conf.Build()
		Expect(err).NotTo(HaveOccurred())
		tls, err := strategy.TLSConfig()
		Expect(err).NotTo(HaveOccurred())
		Expect(tls.InsecureSkipVerify).To(BeTrue())
		Expect(tls.VerifyConnection).NotTo(BeNil())
	})
	It("should build the CA Certs strategy from flags", func() {
		cert := newTestCert()
		flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
		trust.BindFlags(flagset)
		Expect(flagset.Parse([]string{
			"--trust-strategy=cacerts",
			"--cacert=../test/testdata/root_ca.crt",
		})).To(Succeed())

		conf, err := trust.BuildConfigFromFlags(flagset)
		Expect(err).NotTo(HaveOccurred())
		strategy, err := conf.Build()
		Expect(err).NotTo(HaveOccurred())
		tls, err := strategy.TLSConfig()
		Expect(err).NotTo(HaveOccurred())

		Expect(tls.RootCAs.Subjects()).To(HaveLen(1))
		Expect(tls.RootCAs.Subjects()[0]).To(Equal(cert.RawSubject))
	})

	It("should use the system certs if no cacert flag is given", func() {
		flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
		trust.BindFlags(flagset)
		Expect(flagset.Parse([]string{
			"--trust-strategy=cacerts",
		})).To(Succeed())

		conf, err := trust.BuildConfigFromFlags(flagset)
		Expect(err).NotTo(HaveOccurred())
		strategy, err := conf.Build()
		Expect(err).NotTo(HaveOccurred())
		tls, err := strategy.TLSConfig()
		Expect(err).NotTo(HaveOccurred())

		systemCerts, err := x509.SystemCertPool()
		Expect(err).ToNot(HaveOccurred())
		Expect(tls.RootCAs.Subjects()).To(Equal(systemCerts.Subjects()))
	})

	It("should build the insecure strategy from flags", func() {
		flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
		trust.BindFlags(flagset)
		Expect(flagset.Parse([]string{
			"--trust-strategy=insecure",
			"--confirm-insecure",
		})).To(Succeed())

		conf, err := trust.BuildConfigFromFlags(flagset)
		Expect(err).NotTo(HaveOccurred())
		strategy, err := conf.Build()
		Expect(err).NotTo(HaveOccurred())
		tls, err := strategy.TLSConfig()
		Expect(err).NotTo(HaveOccurred())
		Expect(tls.InsecureSkipVerify).To(BeTrue())
		Expect(tls.VerifyConnection).To(BeNil())
	})

	Context("error handling", func() {
		It("should error if no pins are provided for the pkp strategy", func() {
			flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
			trust.BindFlags(flagset)
			Expect(flagset.Parse([]string{
				"--trust-strategy=pkp",
			})).To(Succeed())

			_, err := trust.BuildConfigFromFlags(flagset)
			Expect(err).To(HaveOccurred())
		})

		It("should error if malformed pins are given", func() {
			flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
			trust.BindFlags(flagset)
			Expect(flagset.Parse([]string{
				"--trust-strategy=pkp",
				"--pin=invalid",
			})).To(Succeed())

			_, err := trust.BuildConfigFromFlags(flagset)
			Expect(err).To(HaveOccurred())
		})

		It("should error if invalid certs are given", func() {
			flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
			trust.BindFlags(flagset)
			Expect(flagset.Parse([]string{
				"--trust-strategy=cacerts",
				"--cacert=invalid",
			})).To(Succeed())

			_, err := trust.BuildConfigFromFlags(flagset)
			Expect(err).To(HaveOccurred())

			flagset = pflag.NewFlagSet("test", pflag.ContinueOnError)
			trust.BindFlags(flagset)
			Expect(flagset.Parse([]string{
				"--trust-strategy=cacerts",
				"--cacert=../test/testdata/root_ca.key",
			})).To(Succeed())

			_, err = trust.BuildConfigFromFlags(flagset)
			Expect(err).To(HaveOccurred())
		})

		It("should error if the confirm flag is not passed with insecure", func() {
			flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
			trust.BindFlags(flagset)
			Expect(flagset.Parse([]string{
				"--trust-strategy=insecure",
			})).To(Succeed())

			_, err := trust.BuildConfigFromFlags(flagset)
			Expect(err).To(HaveOccurred())
		})

		It("should error if an invalid trust strategy is given", func() {
			flagset := pflag.NewFlagSet("test", pflag.ContinueOnError)
			trust.BindFlags(flagset)
			Expect(flagset.Parse([]string{
				"--trust-strategy=invalid",
			})).To(Succeed())

			_, err := trust.BuildConfigFromFlags(flagset)
			Expect(err).To(HaveOccurred())
		})
	})
})
