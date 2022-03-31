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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/noauth"
	"github.com/rancher/opni-monitoring/pkg/test"
	"github.com/rancher/opni-monitoring/pkg/util/waitctx"
)

var _ = Describe("Server", Ordered, func() {
	ports, _ := freeport.GetFreePorts(2)
	BeforeAll(func() {
		env := test.Environment{
			TestBin: "../../testbin/bin",
			Logger:  logger.New().Named("test"),
		}
		Expect(env.Start()).To(Succeed())

		client := env.NewManagementClient()
		client.CreateRole(context.Background(), &core.Role{
			Id: "admin",
			MatchLabels: &core.LabelSelector{
				MatchExpressions: []*core.LabelSelectorRequirement{
					{
						Key:      "foo",
						Operator: "Exists",
					},
				},
			},
		})
		client.CreateRoleBinding(context.Background(), &core.RoleBinding{
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
			Logger:                logger.New().Named("test"),
		})
		ctx, ca := context.WithCancel(waitctx.FromContext(context.Background()))
		go func() {
			Expect(srv.Run(ctx)).To(Succeed())
		}()
		DeferCleanup(ca)
	})
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
		}, 5000, 200).Should(Succeed())

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

		values := url.Values{
			"client_id":     []string{"foo"},
			"client_secret": []string{"bar"},
			"code":          []string{code},
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
	})
})
