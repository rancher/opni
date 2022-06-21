package openid_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lestrrat-go/jwx/jwa"
	"github.com/lestrrat-go/jwx/jwt"
	jwtopenid "github.com/lestrrat-go/jwx/jwt/openid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"

	"github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util/waitctx"
)

var _ = Describe("OpenID Middleware", Ordered, test.EnableIfCI[FlakeAttempts](5), Label("temporal"), func() {
	var app *gin.Engine
	Context("no server errors", func() {
		BeforeEach(func() {
			mw, err := openid.New(waitctx.Background(), v1beta1.AuthProviderSpec{
				Type: "openid",
				Options: map[string]any{
					"discovery": map[string]string{
						"issuer": "http://" + discovery.addr,
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())

			app = gin.New()
			app.Use(mw.Handle)
			app.Any("/", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})
		})

		It("should authenticate using an opaque token", func() {
			Eventually(func() error {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Authorization", "Bearer foo")
				recorder := httptest.NewRecorder()
				app.ServeHTTP(recorder, req)
				if recorder.Code != http.StatusOK {
					return fmt.Errorf("unexpected status code: %d", recorder.Code)
				}
				return nil
			}, 2*time.Second, 50*time.Millisecond).Should(Succeed())
		})

		It("should authenticate using an ID token", func() {
			idt, err := jwtopenid.NewBuilder().
				Audience([]string{"foo"}).
				Issuer("http://" + discovery.addr).
				Subject("foo").
				Audience([]string{"test"}).
				Expiration(time.Now().Add(time.Hour)).
				IssuedAt(time.Now()).
				Build()
			Expect(err).NotTo(HaveOccurred())
			token, err := jwt.Sign(idt, jwa.RS256, discovery.key)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() error {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.Header.Set("Authorization", "Bearer "+string(token))
				recorder := httptest.NewRecorder()
				app.ServeHTTP(recorder, req)
				if recorder.Code != http.StatusOK {
					return fmt.Errorf("unexpected status code: %d", recorder.Code)
				}
				return nil
			}, 2*time.Second, 50*time.Millisecond).Should(Succeed())
		})

		When("an id token is signed with the wrong key", func() {
			It("should return http 401", func() {
				idt, err := jwtopenid.NewBuilder().
					Audience([]string{"foo"}).
					Issuer("http://" + discovery.addr).
					Subject("foo").
					Audience([]string{"test"}).
					Expiration(time.Now().Add(time.Hour)).
					IssuedAt(time.Now()).
					Build()
				Expect(err).NotTo(HaveOccurred())
				// wrong signing key
				token, err := jwt.Sign(idt, jwa.RS256, newRandomRS256Key())
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() int {
					req := httptest.NewRequest(http.MethodGet, "/", nil)
					req.Header.Set("Authorization", "Bearer "+string(token))
					recorder := httptest.NewRecorder()
					app.ServeHTTP(recorder, req)
					return recorder.Code
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(http.StatusUnauthorized))
			})
		})

		When("no authentication is provided", func() {
			It("should return http 401", func() {
				Eventually(func() int {
					req := httptest.NewRequest(http.MethodGet, "/", nil)
					recorder := httptest.NewRecorder()
					app.ServeHTTP(recorder, req)
					return recorder.Code
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(http.StatusUnauthorized))
			})
		})
	})
	Context("server or discovery config errors", func() {
		When("the server is unavailable", func() {
			It("should retry until the server becomes available", func() {
				port, err := freeport.GetFreePort()
				Expect(err).NotTo(HaveOccurred())

				mw, err := openid.New(waitctx.Background(), v1beta1.AuthProviderSpec{
					Type: "openid",
					Options: map[string]any{
						"discovery": map[string]string{
							"issuer": fmt.Sprintf("http://localhost:%d", port),
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())

				app := gin.New()
				app.Use(mw.Handle)
				app.Any("/", func(c *gin.Context) {
					c.Status(http.StatusOK)
				})

				Consistently(func() int {
					req := httptest.NewRequest(http.MethodGet, "/", nil)
					recorder := httptest.NewRecorder()
					app.ServeHTTP(recorder, req)
					return recorder.Code
				}, 250*time.Millisecond).Should(Equal(http.StatusServiceUnavailable))

				ctx, ca := context.WithCancel(context.Background())
				defer ca()
				newTestDiscoveryServer(ctx, port)

				Eventually(func() int {
					req := httptest.NewRequest(http.MethodGet, "/", nil)
					req.Header.Set("Authorization", "Bearer foo")
					recorder := httptest.NewRecorder()
					app.ServeHTTP(recorder, req)
					return recorder.Code
				}, 5*time.Second).Should(Equal(http.StatusOK))
			})
		})
		When("an id token is missing the identifying claim", func() {
			It("should return http 401", func() {
				mw, err := openid.New(waitctx.Background(), v1beta1.AuthProviderSpec{
					Type: "openid",
					Options: map[string]any{
						"discovery": map[string]string{
							"issuer": "http://" + discovery.addr,
						},
						"identifyingClaim": "email", // not required for openid tokens
					},
				})
				Expect(err).NotTo(HaveOccurred())

				app := gin.New()
				app.Use(mw.Handle)
				app.Any("/", func(c *gin.Context) {
					c.Status(http.StatusOK)
				})
				listener, err := net.Listen("tcp4", ":0")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(listener.Close)

				go app.RunListener(listener)
				idt, err := jwtopenid.NewBuilder().
					Audience([]string{"foo"}).
					Issuer("http://" + discovery.addr).
					Subject("foo").
					Audience([]string{"test"}).
					Expiration(time.Now().Add(time.Hour)).
					IssuedAt(time.Now()).
					Build()
				Expect(err).NotTo(HaveOccurred())
				token, err := jwt.Sign(idt, jwa.RS256, discovery.key)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() int {
					req := httptest.NewRequest(http.MethodGet, "/", nil)
					req.Header.Set("Authorization", "Bearer "+string(token))
					recorder := httptest.NewRecorder()
					app.ServeHTTP(recorder, req)
					return recorder.Code
				}, 2*time.Second, 50*time.Millisecond).Should(Equal(http.StatusUnauthorized))
			})
		})
	})
})
