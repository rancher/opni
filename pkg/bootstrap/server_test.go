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

	"github.com/gofiber/fiber/v2"
	"github.com/golang/mock/gomock"
	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jws"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/valyala/fasthttp/fasthttputil"

	"github.com/kralicky/opni-monitoring/pkg/bootstrap"
	"github.com/kralicky/opni-monitoring/pkg/ecdh"
	"github.com/kralicky/opni-monitoring/pkg/keyring"
	"github.com/kralicky/opni-monitoring/pkg/logger"
	"github.com/kralicky/opni-monitoring/pkg/storage"
	"github.com/kralicky/opni-monitoring/pkg/test"
	mock_storage "github.com/kralicky/opni-monitoring/pkg/test/mock/storage"
	"github.com/kralicky/opni-monitoring/pkg/tokens"
)

var _ = Describe("Server", func() {
	var token *tokens.Token
	var client *http.Client
	var cert *tls.Certificate
	var app *fiber.App
	var addr string
	var mockTokenStore *mock_storage.MockTokenStore
	var mockTenantStore *mock_storage.MockTenantStore

	BeforeEach(func() {
		token = tokens.NewToken()
		mockTokenStore = mock_storage.NewMockTokenStore(ctrl)
		mockTenantStore = mock_storage.NewMockTenantStore(ctrl)

		mockTokenStore.EXPECT().
			GetToken(gomock.Any(), token.HexID()).
			Return(token, nil).
			AnyTimes()
		mockTokenStore.EXPECT().
			ListTokens(gomock.Any()).
			Return([]*tokens.Token{token}, nil).
			AnyTimes()
		mockTokenStore.EXPECT().
			TokenExists(gomock.Any(), token.HexID()).
			Return(true, nil).
			AnyTimes()
		mockTokenStore.EXPECT().
			TokenExists(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, tokenID string) (bool, error) {
				return tokenID == token.HexID(), nil
			}).
			AnyTimes()

		var storedKeyring keyring.Keyring
		mockKeyringStore := mock_storage.NewMockKeyringStore(ctrl)
		mockKeyringStore.EXPECT().
			Put(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, kr keyring.Keyring) error {
				storedKeyring = kr
				return nil
			}).
			AnyTimes()

		mockKeyringStore.EXPECT().
			Get(gomock.Any()).
			DoAndReturn(func(context.Context) (keyring.Keyring, error) {
				if storedKeyring == nil {
					return nil, storage.ErrNotFound
				}
				return storedKeyring, nil
			}).
			AnyTimes()

		hasCreated := false
		mockTenantStore.EXPECT().
			CreateTenant(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, tenantID string) error {
				hasCreated = true
				return nil
			}).
			AnyTimes()
		mockTenantStore.EXPECT().
			TenantExists(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, tenantID string) (bool, error) {
				return hasCreated, nil
			}).
			AnyTimes()
		mockTenantStore.EXPECT().
			ListTenants(gomock.Any()).
			DoAndReturn(func(context.Context) ([]string, error) {
				if hasCreated {
					return []string{"foo"}, nil
				}
				return []string{}, nil
			}).
			AnyTimes()
		mockTenantStore.EXPECT().
			KeyringStore(gomock.Any(), gomock.Any()).
			Return(mockKeyringStore, nil).
			AnyTimes()
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
		logger.ConfigureApp(app, logger.New().Named("test"))
		server := bootstrap.ServerConfig{
			Certificate: cert,
			TokenStore:  mockTokenStore,
			TenantStore: mockTenantStore,
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

				sig, _ := token.SignDetached(cert.PrivateKey)
				Expect(joinResp.Signatures).To(HaveKeyWithValue(token.HexID(), sig))
			})
		})
		When("no tokens are available", func() {
			BeforeEach(func() {
				mockTokenStore = mock_storage.NewMockTokenStore(ctrl)

				mockTokenStore.EXPECT().
					GetToken(gomock.Any(), gomock.Any()).
					Return(nil, storage.ErrNotFound).
					AnyTimes()

				mockTokenStore.EXPECT().
					ListTokens(gomock.Any()).
					Return([]*tokens.Token{}, nil).
					AnyTimes()

				mockTokenStore.EXPECT().
					TokenExists(gomock.Any(), gomock.Any()).
					Return(false, nil).
					AnyTimes()
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
						jsonData, err := json.Marshal(token)
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
						jsonData, err := json.Marshal(token)
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
					mockTokenStore = mock_storage.NewMockTokenStore(ctrl)

					mockTokenStore.EXPECT().
						GetToken(gomock.Any(), gomock.Any()).
						Return(nil, storage.ErrNotFound).
						AnyTimes()
					mockTokenStore.EXPECT().
						ListTokens(gomock.Any()).
						Return([]*tokens.Token{}, nil).
						AnyTimes()
					mockTokenStore.EXPECT().
						TokenExists(gomock.Any(), gomock.Any()).
						Return(false, nil).
						AnyTimes()
				})
				It("should return http 401", func() {
					jsonData, err := json.Marshal(token)
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
					jsonData, err := json.Marshal(token)
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
