package noauth_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/jwk"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	openidauth "github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/noauth"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util/waitctx"
)

var _ = Describe("Server", Ordered, Label("slow"), func() {
	ports, _ := freeport.GetFreePorts(2)
	BeforeAll(func() {
		env := test.Environment{
			TestBin: "../../testbin/bin",
		}
		Expect(env.Start()).To(Succeed())

		client := env.NewManagementClient()
		client.CreateRole(context.Background(), &corev1.Role{
			Id: "admin",
			MatchLabels: &corev1.LabelSelector{
				MatchExpressions: []*corev1.LabelSelectorRequirement{
					{
						Key:      "foo",
						Operator: "Exists",
					},
				},
			},
		})
		client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:       "admin-rb",
			RoleId:   "admin",
			Subjects: []string{"admin@example.com"},
		})
		addr := env.GatewayConfig().Spec.Management.GRPCListenAddress
		addr = strings.TrimPrefix(addr, "tcp://")

		srv := noauth.NewServer(&noauth.ServerConfig{
			Issuer:                fmt.Sprintf("http://localhost:%d", ports[0]),
			ClientID:              "foo",
			ClientSecret:          "bar",
			RedirectURI:           fmt.Sprintf("http://localhost:%d", ports[1]),
			ManagementAPIEndpoint: addr,
			Port:                  ports[0],
			Logger:                test.Log,
		})
		ctx, ca := context.WithCancel(waitctx.Background())
		go func() {
			defer GinkgoRecover()
			err := srv.Run(ctx)
			Expect(err).To(Or(BeNil(), MatchError(context.Canceled)))
		}()
		DeferCleanup(func() {
			ca()
			waitctx.Wait(ctx, 5*time.Second)
		})
	})
	var accessCode string
	var accessToken string
	It("should issue an access code", func() {
		recv := make(chan *http.Request, 10)
		http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
			recv <- r.Clone(context.Background())
		})
		srv := http.Server{
			Addr: fmt.Sprintf("localhost:%d", ports[1]),
		}
		go func() {
			defer GinkgoRecover()
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				panic(err)
			}
		}()
		defer srv.Close()

		Eventually(func() error {
			_, err := http.Get(fmt.Sprintf("http://localhost:%d", ports[0]) + "/oauth2/authorize?client_id=foo&redirect_uri=http%3A%2F%2Flocalhost%3A" + fmt.Sprint(ports[1]) + "&response_type=code&scope=openid+profile+email&state=1234567890&username=admin%40example.com")
			return err
		}, 5*time.Second, 200*time.Millisecond).Should(Succeed())

		var code string
		select {
		case r := <-recv:
			Expect(r.ParseForm()).To(Succeed())
			code = r.Form.Get("code")
			Expect(code).NotTo(BeEmpty())
			Expect(r.Form.Get("state")).To(Equal("1234567890"))
		case <-time.After(2 * time.Second):
			Fail("timeout")
		}
		accessCode = code
	})
	It("should issue an access token", func() {
		values := url.Values{
			"client_id":     []string{"foo"},
			"client_secret": []string{"bar"},
			"code":          []string{accessCode},
			"grant_type":    []string{"authorization_code"},
			"redirect_uri":  []string{fmt.Sprintf("http://localhost:%d", ports[1])},
		}
		resp, err := http.PostForm(fmt.Sprintf("http://localhost:%d/oauth2/token", ports[0]), values)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		defer resp.Body.Close()
		jsonData, _ := io.ReadAll(resp.Body)
		respData := struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
			ExpiresIn   int    `json:"expires_in"`
			IdToken     string `json:"id_token"`
			Scope       string `json:"scope"`
		}{}
		Expect(json.Unmarshal(jsonData, &respData)).To(Succeed())
		Expect(respData.AccessToken).NotTo(BeEmpty())
		Expect(respData.TokenType).To(Equal("bearer"))
		Expect(respData.ExpiresIn).To(BeNumerically(">", 0))
		Expect(respData.IdToken).NotTo(BeEmpty())
		Expect(respData.Scope).To(Equal("openid profile email"))
		accessToken = respData.AccessToken
	})
	It("should get user info", func() {
		req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/oauth2/userinfo", ports[0]), nil)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Authorization", "Bearer "+accessToken)
		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		defer resp.Body.Close()
		jsonData, _ := io.ReadAll(resp.Body)
		mapClaims := map[string]interface{}{}
		Expect(json.Unmarshal(jsonData, &mapClaims)).To(Succeed())
		Expect(mapClaims["sub"]).To(Equal("admin@example.com"))
	})
	It("should check access token validity", func() {
		req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/oauth2/userinfo", ports[0]), nil)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Authorization", "Bearer foo")
		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

		req, err = http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/oauth2/userinfo", ports[0]), nil)
		Expect(err).NotTo(HaveOccurred())
		resp, err = http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

		req, err = http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/oauth2/userinfo", ports[0]), nil)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Authorization", "Bearer "+accessCode)
		resp, err = http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
	})
	It("should serve the discovery endpoint", func() {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/oauth2/.well-known/openid-configuration", ports[0]))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))
		defer resp.Body.Close()
		jsonData, _ := io.ReadAll(resp.Body)
		wk := openidauth.WellKnownConfiguration{}
		Expect(json.Unmarshal(jsonData, &wk)).To(Succeed())
		Expect(wk.Issuer).To(Equal(fmt.Sprintf("http://localhost:%d/oauth2", ports[0])))
	})
	It("should serve the jwk set", func() {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/oauth2/.well-known/jwks.json", ports[0]))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(Equal("application/jwk-set+json"))
		defer resp.Body.Close()
		jsonData, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		jwks, err := jwk.Parse(jsonData)
		Expect(err).NotTo(HaveOccurred())
		Expect(jwks.Len()).To(Equal(1))
	})
	It("should serve the login page", func() {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/oauth2/authorize?client_id=foo&redirect_uri=http%3A%2F%2Flocalhost%3A"+fmt.Sprint(ports[1])+"&response_type=code&scope=openid+profile+email&state=1234567890", ports[0]))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(Equal("text/html; charset=utf-8"))
		defer resp.Body.Close()
	})
})
