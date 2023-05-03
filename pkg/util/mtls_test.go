package util_test

import (
	"crypto/x509"
	"io/fs"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/util"
)

var _ = Describe("mTLS Utils", Label("unit"), func() {
	It("should load mTLS configurations", func() {
		spec := &v1beta1.MTLSSpec{
			ServerCA:   "../test/testdata/testdata/cortex/root.crt",
			ClientCA:   "../test/testdata/testdata/cortex/root.crt",
			ClientCert: "../test/testdata/testdata/cortex/client.crt",
			ClientKey:  "../test/testdata/testdata/cortex/client.key",
		}
		tlsConfig, err := util.LoadClientMTLSConfig(spec)
		Expect(err).NotTo(HaveOccurred())
		Expect(tlsConfig.RootCAs.Equal(x509.NewCertPool())).To(BeFalse())
		Expect(tlsConfig.ClientCAs.Equal(x509.NewCertPool())).To(BeFalse())
		Expect(tlsConfig.Certificates).To(HaveLen(1))
	})
	When("any of the certificates do not exist", func() {
		It("should error", func() {
			_, err := util.LoadClientMTLSConfig(&v1beta1.MTLSSpec{
				ServerCA:   "../test/testdata/testdata/cortex/_root.crt",
				ClientCA:   "../test/testdata/testdata/cortex/root.crt",
				ClientCert: "../test/testdata/testdata/cortex/client.crt",
				ClientKey:  "../test/testdata/testdata/cortex/client.key",
			})
			Expect(err).To(BeAssignableToTypeOf(&fs.PathError{}))

			_, err = util.LoadClientMTLSConfig(&v1beta1.MTLSSpec{
				ServerCA:   "../test/testdata/testdata/cortex/root.crt",
				ClientCA:   "../test/testdata/testdata/cortex/_root.crt",
				ClientCert: "../test/testdata/testdata/cortex/client.crt",
				ClientKey:  "../test/testdata/testdata/cortex/client.key",
			})
			Expect(err).To(BeAssignableToTypeOf(&fs.PathError{}))

			_, err = util.LoadClientMTLSConfig(&v1beta1.MTLSSpec{
				ServerCA:   "../test/testdata/testdata/cortex/root.crt",
				ClientCA:   "../test/testdata/testdata/cortex/root.crt",
				ClientCert: "../test/testdata/testdata/cortex/_client.crt",
				ClientKey:  "../test/testdata/testdata/cortex/client.key",
			})
			Expect(err).To(BeAssignableToTypeOf(&fs.PathError{}))

			_, err = util.LoadClientMTLSConfig(&v1beta1.MTLSSpec{
				ServerCA:   "../test/testdata/testdata/cortex/root.crt",
				ClientCA:   "../test/testdata/testdata/cortex/root.crt",
				ClientCert: "../test/testdata/testdata/cortex/client.crt",
				ClientKey:  "../test/testdata/testdata/cortex/_client.key",
			})
			Expect(err).To(BeAssignableToTypeOf(&fs.PathError{}))
		})
	})
	When("any of the certificates are malformed", func() {
		It("should error", func() {
			_, err := util.LoadClientMTLSConfig(&v1beta1.MTLSSpec{
				ServerCA:   "../test/testdata/testdata/cortex/root.key",
				ClientCA:   "../test/testdata/testdata/cortex/root.crt",
				ClientCert: "../test/testdata/testdata/cortex/client.crt",
				ClientKey:  "../test/testdata/testdata/cortex/client.key",
			})
			Expect(err).To(MatchError("x509: malformed tbs certificate"))

			_, err = util.LoadClientMTLSConfig(&v1beta1.MTLSSpec{
				ServerCA:   "../test/testdata/testdata/cortex/root.crt",
				ClientCA:   "../test/testdata/testdata/cortex/root.key",
				ClientCert: "../test/testdata/testdata/cortex/client.crt",
				ClientKey:  "../test/testdata/testdata/cortex/client.key",
			})
			Expect(err).To(MatchError("x509: malformed tbs certificate"))

			_, err = util.LoadClientMTLSConfig(&v1beta1.MTLSSpec{
				ServerCA:   "../test/testdata/testdata/cortex/root.crt",
				ClientCA:   "../test/testdata/testdata/cortex/root.crt",
				ClientCert: "../test/testdata/testdata/cortex/client.key",
				ClientKey:  "../test/testdata/testdata/cortex/client.key",
			})
			Expect(err).To(MatchError("tls: failed to find certificate PEM data in certificate input, but did find a private key; PEM inputs may have been switched"))

			_, err = util.LoadClientMTLSConfig(&v1beta1.MTLSSpec{
				ServerCA:   "../test/testdata/testdata/cortex/root.crt",
				ClientCA:   "../test/testdata/testdata/cortex/root.crt",
				ClientCert: "../test/testdata/testdata/cortex/client.crt",
				ClientKey:  "../test/testdata/testdata/cortex/client.crt",
			})
			Expect(err).To(MatchError("tls: found a certificate rather than a key in the PEM for the private key"))
		})
	})
})
