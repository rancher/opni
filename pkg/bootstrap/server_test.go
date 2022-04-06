package bootstrap_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jws"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/valyala/fasthttp/fasthttputil"

	"github.com/rancher/opni-monitoring/pkg/bootstrap"
	"github.com/rancher/opni-monitoring/pkg/capabilities"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/ecdh"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/test"
	"github.com/rancher/opni-monitoring/pkg/tokens"
)

type testCapBackend struct {
	Name       string
	CanInstall bool
}

var _ = Describe("Server", Label(test.Unit, test.Slow), func() {
	var token *core.BootstrapToken
	var token2 *core.BootstrapToken
	var client *http.Client
	var cert *tls.Certificate
	var app *fiber.App
	var addr = new(string)
	var mockTokenStore storage.TokenStore
	var mockClusterStore storage.ClusterStore
	var mockKeyringStoreBroker storage.KeyringStoreBroker
	var testCapBackends []*test.CapabilityInfo

	BeforeEach(func() {
		testCapBackends = append(testCapBackends, &test.CapabilityInfo{
			Name:       "test",
			CanInstall: true,
		})
		ctx, ca := context.WithCancel(context.Background())
		DeferCleanup(ca)
		mockTokenStore = test.NewTestTokenStore(ctx, ctrl)
		mockClusterStore = test.NewTestClusterStore(ctrl)
		mockKeyringStoreBroker = test.NewTestKeyringStoreBroker(ctrl)

		token, _ = mockTokenStore.CreateToken(context.Background(), 1*time.Hour,
			storage.WithLabels(map[string]string{"foo": "bar"}),
		)
		token2, _ = mockTokenStore.CreateToken(context.Background(), 1*time.Hour)
	})
	AfterEach(func() {
		testCapBackends = []*test.CapabilityInfo{}
	})
	JustBeforeEach(func() {
		var err error
		crt, err := tls.X509KeyPair(test.TestData("self_signed_leaf.crt"), test.TestData("self_signed_leaf.key"))
		Expect(err).NotTo(HaveOccurred())
		crt.Leaf, err = x509.ParseCertificate(crt.Certificate[0])
		Expect(err).NotTo(HaveOccurred())
		cert = &crt
		app = fiber.New(fiber.Config{
			DisableStartupMessage: true,
		})
		lg := logger.New().Named("test")
		logger.ConfigureAppLogger(app, "test")
		capBackendStore := capabilities.NewBackendStore(capabilities.ServerInstallerTemplateSpec{}, lg)
		for _, backend := range testCapBackends {
			capBackendStore.Add(backend.Name, test.NewTestCapabilityBackend(ctrl, backend))
		}
		server := bootstrap.ServerConfig{
			CapabilityInstaller: capBackendStore,
			Certificate:         cert,
			TokenStore:          mockTokenStore,
			ClusterStore:        mockClusterStore,
			KeyringStoreBroker:  mockKeyringStoreBroker,
		}
		app.Post("/bootstrap/*", server.Handle)
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{crt},
		}
		app.Server().TLSConfig = tlsConfig
		listener := fasthttputil.NewInmemoryListener()
		*addr = "https://" + listener.Addr().String()
		go app.Listener(listener)
		client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
				DialTLS: func(network, addr string) (net.Conn, error) {
					return listener.Dial()
				},
			},
		}
		DeferCleanup(func() {
			Expect(app.Shutdown()).To(Succeed())
		})
	})

	When("sending a bootstrap join request", func() {
		When("an Authorization header is given", func() {
			It("should return http 400", func() {
				req, err := http.NewRequest("POST", *addr+"/bootstrap/join", nil)
				Expect(err).NotTo(HaveOccurred())
				req.Header.Add("Authorization", "Bearer token")
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			})
		})
		When("no Authorization header is given", func() {
			It("should return the correct bootstrap join response", func() {
				req, err := http.NewRequest("POST", *addr+"/bootstrap/join", nil)
				Expect(err).NotTo(HaveOccurred())
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				body, _ := io.ReadAll(resp.Body)
				joinResp := bootstrap.BootstrapJoinResponse{}
				Expect(json.Unmarshal(body, &joinResp)).To(Succeed())
				Expect(joinResp.Signatures).To(HaveLen(2))

				rawToken, err := tokens.FromBootstrapToken(token)
				Expect(err).NotTo(HaveOccurred())
				sig, _ := rawToken.SignDetached(cert.PrivateKey)
				Expect(joinResp.Signatures).To(HaveKeyWithValue(rawToken.HexID(), sig))
			})
		})
		When("no tokens are available", func() {
			BeforeEach(func() {
				mockTokenStore.DeleteToken(context.Background(), token.Reference())
				mockTokenStore.DeleteToken(context.Background(), token2.Reference())
			})
			It("should return http 409", func() {
				req, err := http.NewRequest("POST", *addr+"/bootstrap/join", nil)
				Expect(err).NotTo(HaveOccurred())
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed))
			})
		})
	})
	When("sending a bootstrap auth request", func() {
		When("an Authorization header is not given", func() {
			It("should return http 401", func() {
				req, err := http.NewRequest("POST", *addr+"/bootstrap/auth", nil)
				Expect(err).NotTo(HaveOccurred())
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			})
		})
		When("an Authorization header is given", func() {
			When("the token is valid", func() {
				When("the client does not send a bootstrap auth request", func() {
					It("should return http 400", func() {
						rawToken, err := tokens.FromBootstrapToken(token)
						Expect(err).NotTo(HaveOccurred())
						jsonData, err := json.Marshal(rawToken)
						Expect(err).NotTo(HaveOccurred())
						sig, err := jws.Sign(jsonData, jwa.EdDSA, cert.PrivateKey)
						Expect(err).NotTo(HaveOccurred())
						req, err := http.NewRequest("POST", *addr+"/bootstrap/auth", nil)
						Expect(err).NotTo(HaveOccurred())
						req.Header.Add("Authorization", "Bearer "+string(sig))
						resp, err := client.Do(req)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
					})
				})
				When("the client sends a bootstrap auth request", func() {
					It("should return http 200", func() {
						rawToken, err := tokens.FromBootstrapToken(token)
						Expect(err).NotTo(HaveOccurred())
						jsonData, err := json.Marshal(rawToken)
						Expect(err).NotTo(HaveOccurred())
						sig, err := jws.Sign(jsonData, jwa.EdDSA, cert.PrivateKey)
						Expect(err).NotTo(HaveOccurred())
						req, err := http.NewRequest("POST", *addr+"/bootstrap/auth", nil)
						Expect(err).NotTo(HaveOccurred())
						req.Header.Add("Authorization", "Bearer "+string(sig))
						ekp := ecdh.NewEphemeralKeyPair()
						authReq := bootstrap.BootstrapAuthRequest{
							Capability:   "test",
							ClientID:     "foo",
							ClientPubKey: ekp.PublicKey,
						}
						j, _ := json.Marshal(authReq)
						req.Header.Set("Content-Type", "application/json")
						req.Body = io.NopCloser(bytes.NewReader(j))
						resp, err := client.Do(req)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusOK))
						token, err := mockTokenStore.GetToken(context.Background(), rawToken.Reference())
						Expect(err).NotTo(HaveOccurred())
						Expect(token.GetMetadata().GetUsageCount()).To(Equal(int64(1)))
						Expect(token.GetMetadata().GetCapabilities()).To(ContainElement(BeEquivalentTo(&core.TokenCapability{
							Type: string(capabilities.JoinExistingCluster),
							Reference: &core.Reference{
								Id: "foo",
							},
						})))
						clusterList, err := mockClusterStore.ListClusters(context.Background(), &core.LabelSelector{}, 0)
						Expect(err).NotTo(HaveOccurred())
						Expect(clusterList.Items).To(HaveLen(1))
						Expect(clusterList.Items[0].GetLabels()).To(HaveKeyWithValue("foo", "bar"))

						By("checking that the cluster's keyring was stored")
						ks, err := mockKeyringStoreBroker.KeyringStore(context.Background(), "gateway", &core.Reference{
							Id: "foo",
						})
						Expect(err).NotTo(HaveOccurred())
						kr, err := ks.Get(context.Background())
						Expect(err).NotTo(HaveOccurred())
						Expect(kr).NotTo(BeNil())

						By("checking that the capability was added to the cluster")
						cluster, err := mockClusterStore.GetCluster(context.Background(), &core.Reference{
							Id: "foo",
						})
						Expect(err).NotTo(HaveOccurred())
						Expect(cluster.GetCapabilities()).To(ContainElement(BeEquivalentTo(&core.ClusterCapability{
							Name: "test",
						})))
					})
				})
			})
			When("the token is invalid", func() {
				It("should return http 401", func() {
					req, err := http.NewRequest("POST", *addr+"/bootstrap/auth", nil)
					Expect(err).NotTo(HaveOccurred())
					req.Header.Add("Authorization", "Bearer invalid")
					resp, err := client.Do(req)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
			When("the token is valid but expired", func() {
				BeforeEach(func() {
					mockTokenStore.DeleteToken(context.Background(), token.Reference())
				})
				It("should return http 401", func() {
					rawToken, err := tokens.FromBootstrapToken(token)
					Expect(err).NotTo(HaveOccurred())
					jsonData, err := json.Marshal(rawToken)
					Expect(err).NotTo(HaveOccurred())
					sig, err := jws.Sign(jsonData, jwa.EdDSA, cert.PrivateKey)
					Expect(err).NotTo(HaveOccurred())
					req, err := http.NewRequest("POST", *addr+"/bootstrap/auth", nil)
					Expect(err).NotTo(HaveOccurred())
					req.Header.Add("Authorization", "Bearer "+string(sig))
					ekp := ecdh.NewEphemeralKeyPair()
					authReq := bootstrap.BootstrapAuthRequest{
						ClientID:     "foo",
						ClientPubKey: ekp.PublicKey,
					}
					j, _ := json.Marshal(authReq)
					req.Header.Set("Content-Type", "application/json")
					req.Body = io.NopCloser(bytes.NewReader(j))
					resp, err := client.Do(req)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			})
			When("joining with an additional capability", func() {
				var ekp ecdh.EphemeralKeyPair
				var newReq func() *http.Request
				JustBeforeEach(func() {
					rawToken, err := tokens.FromBootstrapToken(token)
					Expect(err).NotTo(HaveOccurred())
					jsonData, err := json.Marshal(rawToken)
					Expect(err).NotTo(HaveOccurred())
					sig, err := jws.Sign(jsonData, jwa.EdDSA, cert.PrivateKey)
					Expect(err).NotTo(HaveOccurred())
					newReq = func() *http.Request {
						req, err := http.NewRequest("POST", *addr+"/bootstrap/auth", nil)
						Expect(err).NotTo(HaveOccurred())
						req.Header.Add("Authorization", "Bearer "+string(sig))
						return req
					}
					ekp = ecdh.NewEphemeralKeyPair()

					authReq := bootstrap.BootstrapAuthRequest{
						Capability:   "test",
						ClientID:     "foo",
						ClientPubKey: ekp.PublicKey,
					}

					req := newReq()
					j, _ := json.Marshal(authReq)
					req.Header.Set("Content-Type", "application/json")
					req.Body = io.NopCloser(bytes.NewReader(j))
					resp, err := client.Do(req)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
				})
				When("the requested capability already exists", func() {
					It("should return http 409", func() {
						authReq := bootstrap.BootstrapAuthRequest{
							Capability:   "test",
							ClientID:     "foo",
							ClientPubKey: ekp.PublicKey,
						}

						req := newReq()
						j, _ := json.Marshal(authReq)
						req.Header.Set("Content-Type", "application/json")
						req.Body = io.NopCloser(bytes.NewReader(j))
						resp, err := client.Do(req)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusConflict))
					})
				})
				When("the requested capability does not yet exist", func() {
					BeforeEach(func() {
						testCapBackends = append(testCapBackends,
							&test.CapabilityInfo{
								Name:       "test2",
								CanInstall: true,
							},
							&test.CapabilityInfo{
								Name:       "test3",
								CanInstall: false,
							},
						)
					})
					When("a backend for the capability does not exist", func() {
						It("should return http 404", func() {
							authReq := bootstrap.BootstrapAuthRequest{
								Capability:   "test1.5",
								ClientID:     "foo",
								ClientPubKey: ekp.PublicKey,
							}

							req := newReq()
							j, _ := json.Marshal(authReq)
							req.Header.Set("Content-Type", "application/json")
							req.Body = io.NopCloser(bytes.NewReader(j))
							resp, err := client.Do(req)
							Expect(err).NotTo(HaveOccurred())
							Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
						})
					})
					When("a backend for the capability exists", func() {
						It("should succeed", func() {
							authReq := bootstrap.BootstrapAuthRequest{
								Capability:   "test2",
								ClientID:     "foo",
								ClientPubKey: ekp.PublicKey,
							}

							req := newReq()
							j, _ := json.Marshal(authReq)
							req.Header.Set("Content-Type", "application/json")
							req.Body = io.NopCloser(bytes.NewReader(j))
							resp, err := client.Do(req)
							Expect(err).NotTo(HaveOccurred())
							Expect(resp.StatusCode).To(Equal(http.StatusOK))
						})
					})
					When("the capability cannot be installed", func() {
						It("should return http 503", func() {
							authReq := bootstrap.BootstrapAuthRequest{
								Capability:   "test3",
								ClientID:     "foo",
								ClientPubKey: ekp.PublicKey,
							}

							req := newReq()
							j, _ := json.Marshal(authReq)
							req.Header.Set("Content-Type", "application/json")
							req.Body = io.NopCloser(bytes.NewReader(j))
							resp, err := client.Do(req)
							Expect(err).NotTo(HaveOccurred())
							Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
						})
					})
					When("the token used does not have the correct join capability", func() {
						It("should return http 401", func() {
							rawToken2, err := tokens.FromBootstrapToken(token2)
							Expect(err).NotTo(HaveOccurred())
							jsonData, err := json.Marshal(rawToken2)
							Expect(err).NotTo(HaveOccurred())
							sig, err := jws.Sign(jsonData, jwa.EdDSA, cert.PrivateKey)
							Expect(err).NotTo(HaveOccurred())

							req, err := http.NewRequest("POST", *addr+"/bootstrap/auth", nil)
							Expect(err).NotTo(HaveOccurred())
							req.Header.Add("Authorization", "Bearer "+string(sig))

							authReq := bootstrap.BootstrapAuthRequest{
								Capability:   "test2",
								ClientID:     "foo",
								ClientPubKey: ekp.PublicKey,
							}
							j, _ := json.Marshal(authReq)
							req.Header.Set("Content-Type", "application/json")
							req.Body = io.NopCloser(bytes.NewReader(j))
							resp, err := client.Do(req)
							Expect(err).NotTo(HaveOccurred())
							Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
						})
					})
				})
			})
		})
	})
	When("sending a request to an invalid path", func() {
		It("should return http 404", func() {
			req, err := http.NewRequest("POST", *addr+"/bootstrap/foo", nil)
			Expect(err).NotTo(HaveOccurred())
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})
})
