package keyring_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/atomic"

	"github.com/kralicky/opni-gateway/pkg/keyring"
)

var (
	testCertificate = `-----BEGIN CERTIFICATE-----
MIIBHjCB0aADAgECAhAOWE+Vhh8kSITCq+6EhdIkMAUGAytlcDAOMQwwCgYDVQQD
EwNmb28wHhcNMjIwMTA2MDQyOTI2WhcNMzIwMTA0MDQyOTI2WjAOMQwwCgYDVQQD
EwNmb28wKjAFBgMrZXADIQDqiNw97n9I1yW+uNuN9UHfhmC00JgCoccEk/oBfleE
pqNFMEMwDgYDVR0PAQH/BAQDAgEGMBIGA1UdEwEB/wQIMAYBAf8CAQEwHQYDVR0O
BBYEFL4nnF44+S9a+64CqaAwFlzLIIS4MAUGAytlcANBAOQWH4miLHMugDoOFZ1N
71E+OzHO9LvbOrXVYivr4uuP+2IZmiZjklw/ZMozYfDq2AIE2Uft1FXLnh4hBoSH
YgE=
-----END CERTIFICATE-----
`
)

var _ = Describe("Keyring", func() {
	When("creating an empty keyring", func() {
		It("should function correctly", func() {
			By("creating a new keyring")
			kr := keyring.New()
			Expect(kr).NotTo(BeNil())

			By("ensuring functions passed to Try are never called")
			counter := atomic.NewInt32(0)
			kr.Try(func(*keyring.SharedKeys) {
				counter.Inc()
			}, func(*keyring.TLSKey) {
				counter.Inc()
			})
			Expect(counter.Load()).To(Equal(int32(0)))

			By("ensuring ForEach is never called")
			counter.Store(0)
			kr.ForEach(func(key interface{}) {
				counter.Inc()
			})
			Expect(counter.Load()).To(Equal(int32(0)))

			By("ensuring Marshal returns an empty object")
			j, err := kr.Marshal()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(j)).To(Equal("{}"))
		})
	})
	When("creating a keyring with one key", func() {
		It("should function correctly", func() {
			By("creating a new keyring")
			kr := keyring.New(keyring.NewTLSKeys(&keyring.TLSConfig{
				ServerName:       "foo",
				CurvePreferences: []tls.CurveID{tls.X25519},
			}))
			Expect(kr).NotTo(BeNil())

			By("ensuring Try calls the correct function")
			counter := atomic.NewInt32(0)
			kr.Try(func(keys *keyring.SharedKeys) {
				Fail("Try called the wrong function")
			}, func(key *keyring.TLSKey) {
				counter.Inc()
				Expect(key.TLSConfig.ServerName).To(Equal("foo"))
				Expect(key.TLSConfig.CurvePreferences).To(Equal([]tls.CurveID{tls.X25519}))
			})
			Expect(counter.Load()).To(Equal(int32(1)))

			By("ensuring ForEach is called once")
			counter.Store(0)
			kr.ForEach(func(key interface{}) {
				counter.Inc()
				Expect(key).To(BeAssignableToTypeOf(&keyring.TLSKey{}))
				Expect(key.(*keyring.TLSKey).TLSConfig.ServerName).To(Equal("foo"))
				Expect(key.(*keyring.TLSKey).TLSConfig.CurvePreferences).To(Equal([]tls.CurveID{tls.X25519}))
			})
			Expect(counter.Load()).To(Equal(int32(1)))

			By("ensuring Marshal returns the correct object")
			j, err := kr.Marshal()
			Expect(err).NotTo(HaveOccurred())
			jsonString := `{"tlsKey":{"tlsConfig":{"curvePreferences":[29],"serverName":"foo"}}}`
			Expect(string(j)).To(Equal(jsonString))

			By("ensuring Unmarshal returns the correct object")
			kr2, err := keyring.Unmarshal([]byte(jsonString))
			Expect(err).NotTo(HaveOccurred())
			Expect(kr2).NotTo(BeNil())
			Expect(kr).To(BeEquivalentTo(kr2))
		})
	})
	When("creating a keyring with multiple keys", func() {
		It("should function correctly", func() {
			certBlock, rest := pem.Decode([]byte(testCertificate))
			Expect(certBlock).NotTo(BeNil())
			Expect(rest).To(BeEmpty())
			cert, err := x509.ParseCertificate(certBlock.Bytes)
			Expect(err).NotTo(HaveOccurred())

			By("creating a new keyring")
			kr := keyring.New(keyring.NewTLSKeys(&keyring.TLSConfig{
				ServerName:       "foo",
				CurvePreferences: []tls.CurveID{tls.X25519},
				RootCAs:          [][]byte{cert.Raw},
			}), keyring.NewSharedKeys(make([]byte, 64)))
			Expect(kr).NotTo(BeNil())

			By("ensuring Try calls all functions")
			counterA := atomic.NewInt32(0)
			counterB := atomic.NewInt32(0)
			kr.Try(func(keys *keyring.SharedKeys) {
				counterA.Inc()
				Expect(keys.ClientKey).To(HaveLen(64))
				Expect(keys.ServerKey).To(HaveLen(64))
			}, func(key *keyring.TLSKey) {
				counterB.Inc()
				Expect(key.TLSConfig.ServerName).To(Equal("foo"))
				Expect(key.TLSConfig.CurvePreferences).To(Equal([]tls.CurveID{tls.X25519}))
			})
			Expect(counterA.Load()).To(Equal(int32(1)))
			Expect(counterB.Load()).To(Equal(int32(1)))

			By("ensuring ForEach is called for each key")
			counter := atomic.NewInt32(0)
			kr.ForEach(func(key interface{}) {
				counter.Inc()
			})
			Expect(counter.Load()).To(Equal(int32(2)))

			By("ensuring Marshal followed by Unmarshal returns the same data")
			data, err := kr.Marshal()
			Expect(err).NotTo(HaveOccurred())
			kr2, err := keyring.Unmarshal(data)
			Expect(err).NotTo(HaveOccurred())
			Expect(kr2).NotTo(BeNil())
			Expect(kr).To(BeEquivalentTo(kr2))
		})
	})
	It("should handle errors", func() {
		Expect(func() {
			keyring.New("not_an_allowed_keytype")
		}).To(PanicWith(keyring.ErrInvalidKeyType))
		kr := keyring.New()
		Expect(func() {
			kr.Try("not_a_function")
		}).To(PanicWith("invalid UseKeyFn"))
		Expect(func() {
			kr.Try(func(a, b string) {})
		}).To(PanicWith("invalid UseKeyFn (requires one parameter)"))
		kr, err := keyring.Unmarshal([]byte("not_json"))
		Expect(kr).To(BeNil())
		Expect(err).To(HaveOccurred())
		Expect(func() {
			keyring.NewSharedKeys([]byte("not_64_bytes"))
		}).To(PanicWith("shared secret must be 64 bytes"))
	})
	It("should convert between custom TLS config types", func() {
		block, _ := pem.Decode([]byte(testCertificate))
		tlsConfig := &keyring.TLSConfig{
			ServerName:       "foo",
			CurvePreferences: []tls.CurveID{tls.X25519},
			RootCAs:          [][]byte{block.Bytes},
		}
		cryptoTlsConfig := tlsConfig.ToCryptoTLSConfig()
		Expect(cryptoTlsConfig.ServerName).To(Equal(tlsConfig.ServerName))
		Expect(cryptoTlsConfig.CurvePreferences).To(Equal(tlsConfig.CurvePreferences))
		Expect(len(cryptoTlsConfig.RootCAs.Subjects())).To(Equal(len(tlsConfig.RootCAs)))
	})
})
