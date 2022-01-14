package bootstrap_test

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jws"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kralicky/opni-monitoring/pkg/bootstrap"
	"github.com/kralicky/opni-monitoring/pkg/ecdh"
	"github.com/kralicky/opni-monitoring/pkg/ident"
	"github.com/kralicky/opni-monitoring/pkg/pkp"
	"github.com/kralicky/opni-monitoring/pkg/test"
	mock_ident "github.com/kralicky/opni-monitoring/pkg/test/mock/ident"
	"github.com/kralicky/opni-monitoring/pkg/tokens"
)

type errProvider struct{}

func (p *errProvider) UniqueIdentifier(context.Context) (string, error) {
	return "", errors.New("test")
}

var _ = Describe("Client", func() {
	token := tokens.NewToken()
	var fooIdent ident.Provider
	var cert *tls.Certificate

	Specify("setup", func() {
		mockIdent := mock_ident.NewMockProvider(ctrl)
		mockIdent.EXPECT().
			UniqueIdentifier(gomock.Any()).
			Return("foo", nil).
			AnyTimes()
		fooIdent = mockIdent
		var err error
		crt, err := tls.X509KeyPair(test.TestData("self_signed_leaf.crt"), test.TestData("self_signed_leaf.key"))
		Expect(err).NotTo(HaveOccurred())
		crt.Leaf, err = x509.ParseCertificate(crt.Certificate[0])
		Expect(err).NotTo(HaveOccurred())
		cert = &crt
	})
	It("should bootstrap with the server", func() {
		mux := http.NewServeMux()

		mux.HandleFunc("/bootstrap/join", func(rw http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			Expect(r.Header.Get("Authorization")).To(BeEmpty())
			data, err := token.SignDetached(cert.PrivateKey)
			Expect(err).NotTo(HaveOccurred())
			rw.WriteHeader(http.StatusOK)
			j, _ := json.Marshal(bootstrap.BootstrapJoinResponse{
				Signatures: map[string][]byte{
					token.HexID(): data,
				},
			})
			_, err = rw.Write(j)
			Expect(err).NotTo(HaveOccurred())
		})
		mux.HandleFunc("/bootstrap/auth", func(rw http.ResponseWriter, r *http.Request) {
			defer GinkgoRecover()
			Expect(r.Header.Get("Authorization")).NotTo(BeEmpty())
			authHeader := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

			_, err := jws.Verify([]byte(authHeader), jwa.EdDSA, cert.PrivateKey.(ed25519.PrivateKey).Public())
			Expect(err).NotTo(HaveOccurred())

			req := bootstrap.BootstrapAuthRequest{}
			Expect(json.NewDecoder(r.Body).Decode(&req)).To(Succeed())
			Expect(req.ClientID).To(Equal("foo"))
			Expect(req.ClientPubKey).To(HaveLen(32))

			ekp := ecdh.NewEphemeralKeyPair()
			rw.WriteHeader(http.StatusOK)
			resp, _ := json.Marshal(bootstrap.BootstrapAuthResponse{
				ServerPubKey: ekp.PublicKey,
			})
			_, err = rw.Write(resp)
			Expect(err).NotTo(HaveOccurred())
		})
		server := httptest.NewUnstartedServer(mux)
		server.TLS = &tls.Config{
			Certificates: []tls.Certificate{*cert},
		}
		server.StartTLS()
		defer server.Close()

		cc := bootstrap.ClientConfig{
			Token:    token,
			Pins:     []*pkp.PublicKeyPin{pkp.NewSha256(server.Certificate())},
			Endpoint: server.URL,
		}

		_, err := cc.Bootstrap(context.Background(), fooIdent)
		Expect(err).NotTo(HaveOccurred())
	})
	Context("error handling", func() {
		When("an invalid endpoint is given", func() {
			It("should error", func() {
				cc := bootstrap.ClientConfig{
					Token:    token,
					Pins:     []*pkp.PublicKeyPin{pkp.NewSha256(cert.Leaf)},
					Endpoint: "\x7f",
				}
				kr, err := cc.Bootstrap(context.Background(), fooIdent)
				Expect(kr).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("net/url"))
			})
		})
		When("invalid pins are given", func() {
			It("should error", func() {
				cc := bootstrap.ClientConfig{
					Token:    token,
					Pins:     []*pkp.PublicKeyPin{},
					Endpoint: "https://localhost",
				}
				kr, err := cc.Bootstrap(context.Background(), fooIdent)
				Expect(kr).To(BeNil())
				Expect(err).To(MatchError(pkp.ErrNoPins))
			})
		})
		When("the client fails to send a request to the server", func() {
			It("should error", func() {
				cc := bootstrap.ClientConfig{
					Token:    token,
					Pins:     []*pkp.PublicKeyPin{pkp.NewSha256(cert.Leaf)},
					Endpoint: "https://localhost:65545",
				}
				kr, err := cc.Bootstrap(context.Background(), fooIdent)
				Expect(kr).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("invalid port"))
			})
		})
		When("the server returns a non-200 error code", func() {
			It("should error", func() {
				server := httptest.NewTLSServer(http.NewServeMux())
				defer server.Close()
				cc := bootstrap.ClientConfig{
					Token:    token,
					Pins:     []*pkp.PublicKeyPin{pkp.NewSha256(server.Certificate())},
					Endpoint: server.URL,
				}
				kr, err := cc.Bootstrap(context.Background(), fooIdent)
				Expect(kr).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("404"))
			})
		})
		When("the server returns invalid response data", func() {
			It("should error", func() {
				mux := http.NewServeMux()
				mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
					rw.WriteHeader(http.StatusOK)
				})
				server := httptest.NewTLSServer(mux)
				defer server.Close()
				cc := bootstrap.ClientConfig{
					Token:    token,
					Pins:     []*pkp.PublicKeyPin{pkp.NewSha256(server.Certificate())},
					Endpoint: server.URL,
				}
				kr, err := cc.Bootstrap(context.Background(), fooIdent)
				Expect(kr).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("unexpected end of JSON input"))
			})
		})
		When("the client can't find a matching signature sent by the server", func() {
			It("should error", func() {
				mux := http.NewServeMux()
				mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
					rw.WriteHeader(http.StatusOK)
					j, _ := json.Marshal(bootstrap.BootstrapJoinResponse{
						Signatures: make(map[string][]byte),
					})
					_, err := rw.Write(j)
					Expect(err).NotTo(HaveOccurred())
				})
				server := httptest.NewTLSServer(mux)
				defer server.Close()
				cc := bootstrap.ClientConfig{
					Token:    token,
					Pins:     []*pkp.PublicKeyPin{pkp.NewSha256(server.Certificate())},
					Endpoint: server.URL,
				}
				kr, err := cc.Bootstrap(context.Background(), fooIdent)
				Expect(kr).To(BeNil())
				Expect(err).To(MatchError(bootstrap.ErrNoValidSignature))
			})
		})
		When("the client can't find a unique identifier", func() {
			It("should error", func() {
				mux := http.NewServeMux()

				mux.HandleFunc("/bootstrap/join", func(rw http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()
					Expect(r.Header.Get("Authorization")).To(BeEmpty())
					data, err := token.SignDetached(cert.PrivateKey)
					Expect(err).NotTo(HaveOccurred())
					rw.WriteHeader(http.StatusOK)
					j, _ := json.Marshal(bootstrap.BootstrapJoinResponse{
						Signatures: map[string][]byte{
							token.HexID(): data,
						},
					})
					_, err = rw.Write(j)
					Expect(err).NotTo(HaveOccurred())
				})
				server := httptest.NewUnstartedServer(mux)
				server.TLS = &tls.Config{
					Certificates: []tls.Certificate{*cert},
				}
				server.StartTLS()
				defer server.Close()

				cc := bootstrap.ClientConfig{
					Token:    token,
					Pins:     []*pkp.PublicKeyPin{pkp.NewSha256(server.Certificate())},
					Endpoint: server.URL,
				}

				_, err := cc.Bootstrap(context.Background(), &errProvider{})
				Expect(err).To(MatchError("failed to obtain unique identifier: test"))
			})
		})
		When("the bootstrap auth request fails", func() {
			It("should error", func() {
				mux := http.NewServeMux()

				mux.HandleFunc("/bootstrap/join", func(rw http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()
					Expect(r.Header.Get("Authorization")).To(BeEmpty())
					data, err := token.SignDetached(cert.PrivateKey)
					Expect(err).NotTo(HaveOccurred())
					rw.WriteHeader(http.StatusOK)
					j, _ := json.Marshal(bootstrap.BootstrapJoinResponse{
						Signatures: map[string][]byte{
							token.HexID(): data,
						},
					})
					_, err = rw.Write(j)
					Expect(err).NotTo(HaveOccurred())
				})
				mux.HandleFunc("/bootstrap/auth", func(rw http.ResponseWriter, r *http.Request) {
					rw.WriteHeader(http.StatusInternalServerError)
				})
				server := httptest.NewUnstartedServer(mux)
				server.TLS = &tls.Config{
					Certificates: []tls.Certificate{*cert},
				}
				server.StartTLS()
				defer server.Close()

				cc := bootstrap.ClientConfig{
					Token:    token,
					Pins:     []*pkp.PublicKeyPin{pkp.NewSha256(server.Certificate())},
					Endpoint: server.URL,
				}

				_, err := cc.Bootstrap(context.Background(), fooIdent)
				Expect(err).To(MatchError("bootstrap failed: 500 Internal Server Error"))
			})
		})
		When("the server sends invalid response data", func() {
			It("should error", func() {
				mux := http.NewServeMux()

				mux.HandleFunc("/bootstrap/join", func(rw http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()
					Expect(r.Header.Get("Authorization")).To(BeEmpty())
					data, err := token.SignDetached(cert.PrivateKey)
					Expect(err).NotTo(HaveOccurred())
					rw.WriteHeader(http.StatusOK)
					j, _ := json.Marshal(bootstrap.BootstrapJoinResponse{
						Signatures: map[string][]byte{
							token.HexID(): data,
						},
					})
					_, err = rw.Write(j)
					Expect(err).NotTo(HaveOccurred())
				})
				mux.HandleFunc("/bootstrap/auth", func(rw http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()
					rw.WriteHeader(http.StatusOK)
					_, err := rw.Write([]byte("$"))
					Expect(err).NotTo(HaveOccurred())
				})
				server := httptest.NewUnstartedServer(mux)
				server.TLS = &tls.Config{
					Certificates: []tls.Certificate{*cert},
				}
				server.StartTLS()
				defer server.Close()

				cc := bootstrap.ClientConfig{
					Token:    token,
					Pins:     []*pkp.PublicKeyPin{pkp.NewSha256(server.Certificate())},
					Endpoint: server.URL,
				}

				_, err := cc.Bootstrap(context.Background(), fooIdent)
				Expect(err.Error()).To(ContainSubstring("invalid character"))

			})
		})
		When("the server sends an invalid public key", func() {
			It("should error", func() {
				mux := http.NewServeMux()

				mux.HandleFunc("/bootstrap/join", func(rw http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()
					Expect(r.Header.Get("Authorization")).To(BeEmpty())
					data, err := token.SignDetached(cert.PrivateKey)
					Expect(err).NotTo(HaveOccurred())
					rw.WriteHeader(http.StatusOK)
					j, _ := json.Marshal(bootstrap.BootstrapJoinResponse{
						Signatures: map[string][]byte{
							token.HexID(): data,
						},
					})
					_, err = rw.Write(j)
					Expect(err).NotTo(HaveOccurred())
				})
				mux.HandleFunc("/bootstrap/auth", func(rw http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()
					rw.WriteHeader(http.StatusOK)
					j, _ := json.Marshal(bootstrap.BootstrapAuthResponse{
						ServerPubKey: []byte("invalid"),
					})
					_, err := rw.Write(j)
					Expect(err).NotTo(HaveOccurred())
				})
				server := httptest.NewUnstartedServer(mux)
				server.TLS = &tls.Config{
					Certificates: []tls.Certificate{*cert},
				}
				server.StartTLS()
				defer server.Close()

				cc := bootstrap.ClientConfig{
					Token:    token,
					Pins:     []*pkp.PublicKeyPin{pkp.NewSha256(server.Certificate())},
					Endpoint: server.URL,
				}

				_, err := cc.Bootstrap(context.Background(), fooIdent)
				Expect(err.Error()).To(ContainSubstring("bad point length"))
			})

		})
	})
})
