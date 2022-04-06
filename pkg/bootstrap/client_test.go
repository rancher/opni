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
	"os"
	"runtime"
	"strings"

	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jws"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	"github.com/rancher/opni-monitoring/pkg/bootstrap"
	"github.com/rancher/opni-monitoring/pkg/config/meta"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/ecdh"
	"github.com/rancher/opni-monitoring/pkg/ident"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/pkp"
	"github.com/rancher/opni-monitoring/pkg/test"
	"github.com/rancher/opni-monitoring/pkg/tokens"
)

type errProvider struct{}

func (p *errProvider) UniqueIdentifier(context.Context) (string, error) {
	return "", errors.New("test")
}

var _ = Describe("Client", Ordered, Label(test.Unit, test.Slow), func() {
	token := tokens.NewToken()
	var fooIdent ident.Provider
	var cert *tls.Certificate

	BeforeAll(func() {
		if runtime.GOOS != "linux" {
			Skip("skipping test on non-linux OS")
		}
		fooIdent = test.NewTestIdentProvider(ctrl, "foo")
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
	When("the bootstrap process is complete", func() {
		It("should erase bootstrap tokens from the config secret", func() {
			if runtime.GOOS != "linux" {
				Skip("skipping test on non-linux OS")
			}
			env := test.Environment{
				TestBin: "../../testbin/bin",
				Logger:  logger.New().Named("test"),
			}
			k8sConfig, err := env.StartK8s()
			Expect(err).NotTo(HaveOccurred())

			os.Setenv("POD_NAMESPACE", "default")
			client, err := kubernetes.NewForConfig(k8sConfig)
			Expect(err).NotTo(HaveOccurred())

			config := v1beta1.AgentConfig{
				TypeMeta: meta.TypeMeta{
					APIVersion: "v1beta1",
					Kind:       "AgentConfig",
				},
				Spec: v1beta1.AgentConfigSpec{
					IdentityProvider: "foo",
					Bootstrap: &v1beta1.BootstrapSpec{
						Token: "foo",
						Pins:  []string{"foo", "bar"},
					},
				},
			}
			data, err := yaml.Marshal(config)
			Expect(err).NotTo(HaveOccurred())
			_, err = client.CoreV1().Secrets("default").
				Create(context.Background(), &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "agent-config",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"config.yaml": data,
					},
				}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			cc := bootstrap.ClientConfig{
				K8sConfig:    k8sConfig,
				K8sNamespace: "default",
			}
			err = cc.Finalize(context.Background())
			Expect(err).NotTo(HaveOccurred())

			secret, err := client.CoreV1().Secrets("default").
				Get(context.Background(), "agent-config", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			erased := v1beta1.AgentConfig{}
			Expect(yaml.Unmarshal(secret.Data["config.yaml"], &erased)).To(Succeed())
			Expect(erased.Spec.IdentityProvider).To(Equal("foo"))
			Expect(erased.Spec.Bootstrap).To(BeNil())

			Expect(env.Stop()).To(Succeed())
		})
		When("the defaults are used and the in-cluster config is unavailable", func() {
			It("should error", func() {
				// sanity check, this is set above
				Expect(os.Getenv("POD_NAMESPACE")).To(Equal("default"))

				cc := bootstrap.ClientConfig{
					K8sConfig: nil,
				}
				err := cc.Finalize(context.Background())
				Expect(err).To(MatchError(rest.ErrNotInCluster))
			})
		})
		When("a namespace is not found or configured", func() {
			It("should error", func() {
				// This is set above
				os.Unsetenv("POD_NAMESPACE")

				cc := bootstrap.ClientConfig{
					K8sConfig:    nil,
					K8sNamespace: "",
				}
				err := cc.Finalize(context.Background())
				Expect(err).To(MatchError("POD_NAMESPACE not set, and no namespace was explicitly configured"))
			})
		})
	})
	Context("error handling", func() {
		When("no token is given", func() {
			It("should error", func() {
				cc := bootstrap.ClientConfig{}
				kr, err := cc.Bootstrap(context.Background(), fooIdent)
				Expect(kr).To(BeNil())
				Expect(err).To(MatchError(bootstrap.ErrNoToken))
			})
		})
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
