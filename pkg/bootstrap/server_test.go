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
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/ecdh"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/test"
	"github.com/rancher/opni-monitoring/pkg/tokens"
)

var _ = Describe("Server", func() {
	var token *core.BootstrapToken
	var client *http.Client
	var cert *tls.Certificate
	var app *fiber.App
	var addr string
	var mockTokenStore storage.TokenStore
	var mockClusterStore storage.ClusterStore
	var mockKeyringStoreBroker storage.KeyringStoreBroker

	BeforeEach(func() {
		ctx, ca := context.WithCancel(context.Background())
		DeferCleanup(ca)
		mockTokenStore = test.NewTestTokenStore(ctx, ctrl)
		mockClusterStore = test.NewTestClusterStore(ctrl)
		mockKeyringStoreBroker = test.NewTestKeyringStoreBroker(ctrl)

		token, _ = mockTokenStore.CreateToken(context.Background(), 1*time.Hour,
			map[string]string{"foo": "bar"})
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
		logger.ConfigureAppLogger(app, "test")
		server := bootstrap.ServerConfig{
			Certificate:        cert,
			TokenStore:         mockTokenStore,
			ClusterStore:       mockClusterStore,
			KeyringStoreBroker: mockKeyringStoreBroker,
		}
		app.Post("/bootstrap/*", server.Handle)
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{crt},
		}
		app.Server().TLSConfig = tlsConfig
		listener := fasthttputil.NewInmemoryListener()
		addr = "https://" + listener.Addr().String()
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
				req, err := http.NewRequest("POST", addr+"/bootstrap/join", nil)
				Expect(err).NotTo(HaveOccurred())
				req.Header.Add("Authorization", "Bearer token")
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			})
		})
		When("no Authorization header is given", func() {
			It("should return the correct bootstrap join response", func() {
				req, err := http.NewRequest("POST", addr+"/bootstrap/join", nil)
				Expect(err).NotTo(HaveOccurred())
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				body, _ := io.ReadAll(resp.Body)
				joinResp := bootstrap.BootstrapJoinResponse{}
				Expect(json.Unmarshal(body, &joinResp)).To(Succeed())
				Expect(joinResp.Signatures).To(HaveLen(1))

				rawToken, err := tokens.FromBootstrapToken(token)
				Expect(err).NotTo(HaveOccurred())
				sig, _ := rawToken.SignDetached(cert.PrivateKey)
				Expect(joinResp.Signatures).To(HaveKeyWithValue(rawToken.HexID(), sig))
			})
		})
		When("no tokens are available", func() {
			BeforeEach(func() {
				mockTokenStore.DeleteToken(context.Background(), token.Reference())
			})
			It("should return http 409", func() {
				req, err := http.NewRequest("POST", addr+"/bootstrap/join", nil)
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
				req, err := http.NewRequest("POST", addr+"/bootstrap/auth", nil)
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
						req, err := http.NewRequest("POST", addr+"/bootstrap/auth", nil)
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
						req, err := http.NewRequest("POST", addr+"/bootstrap/auth", nil)
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
						Expect(resp.StatusCode).To(Equal(http.StatusOK))
						Expect(token.GetMetadata().GetUsageCount()).To(Equal(int64(1)))
						clusterList, err := mockClusterStore.ListClusters(context.Background(), &core.LabelSelector{}, 0)
						Expect(err).NotTo(HaveOccurred())
						Expect(clusterList.Items).To(HaveLen(1))
						Expect(clusterList.Items[0].Labels).To(HaveKeyWithValue("foo", "bar"))

						By("checking that the cluster's keyring was stored")
						ks, err := mockKeyringStoreBroker.KeyringStore(context.Background(), "gateway", &core.Reference{
							Id: "foo",
						})
						Expect(err).NotTo(HaveOccurred())
						kr, err := ks.Get(context.Background())
						Expect(err).NotTo(HaveOccurred())
						Expect(kr).NotTo(BeNil())
					})
				})
			})
			When("the token is invalid", func() {
				It("should return http 401", func() {
					req, err := http.NewRequest("POST", addr+"/bootstrap/auth", nil)
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
					req, err := http.NewRequest("POST", addr+"/bootstrap/auth", nil)
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
			When("the client ID already exists", func() {
				It("should return http 409", func() {
					rawToken, err := tokens.FromBootstrapToken(token)
					Expect(err).NotTo(HaveOccurred())
					jsonData, err := json.Marshal(rawToken)
					Expect(err).NotTo(HaveOccurred())
					sig, err := jws.Sign(jsonData, jwa.EdDSA, cert.PrivateKey)
					Expect(err).NotTo(HaveOccurred())
					req, err := http.NewRequest("POST", addr+"/bootstrap/auth", nil)
					Expect(err).NotTo(HaveOccurred())
					req.Header.Add("Authorization", "Bearer "+string(sig))
					ekp := ecdh.NewEphemeralKeyPair()
					authReq := bootstrap.BootstrapAuthRequest{
						ClientID:     "foo",
						ClientPubKey: ekp.PublicKey,
					}

					{
						reqCopy := req.Clone(req.Context())
						j, _ := json.Marshal(authReq)
						reqCopy.Header.Set("Content-Type", "application/json")
						reqCopy.Body = io.NopCloser(bytes.NewReader(j))
						resp, err := client.Do(reqCopy)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusOK))
					}
					{
						reqCopy := req.Clone(context.Background())
						j, _ := json.Marshal(authReq)
						reqCopy.Header.Set("Content-Type", "application/json")
						reqCopy.Body = io.NopCloser(bytes.NewReader(j))
						resp, err := client.Do(reqCopy)
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusConflict))
					}
				})
			})
		})
	})
	When("sending a request to an invalid path", func() {
		It("should return http 404", func() {
			req, err := http.NewRequest("POST", addr+"/bootstrap/foo", nil)
			Expect(err).NotTo(HaveOccurred())
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})
})
