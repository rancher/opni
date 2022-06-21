package e2e

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/ini.v1"
	k8scorev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Gateway Test", Ordered, Label("e2e", "slow"), func() {
	var mainClusterRef *corev1.Reference
	It("should connect to the management server", func() {
		resp, err := mgmtClient.CertsInfo(context.Background(), &emptypb.Empty{})
		fmt.Println(resp.String())
		Expect(err).NotTo(HaveOccurred())

		var kubeSystem k8scorev1.Namespace
		err = k8sClient.Get(context.Background(), types.NamespacedName{Name: "kube-system"}, &kubeSystem)
		Expect(err).NotTo(HaveOccurred())
		mainClusterRef = &corev1.Reference{
			Id: string(kubeSystem.UID),
		}
	})
	It("should be running an agent inside the main cluster", func() {
		l, err := mgmtClient.ListClusters(context.Background(), &managementv1.ListClustersRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(l.Items).To(HaveLen(1))

		Expect(l.Items[0].Id).To(Equal(mainClusterRef.Id))
	})
	Specify("the agent should be healthy", func() {
		Eventually(func() string {
			hs, err := mgmtClient.GetClusterHealthStatus(context.Background(), mainClusterRef)
			if err != nil {
				return err.Error()
			}
			if !hs.Status.Connected {
				return "not connected"
			}
			if len(hs.Health.Conditions) > 0 {
				return strings.Join(hs.Health.Conditions, ", ")
			}
			if !hs.Health.Ready {
				return "not ready"
			}
			return "ok"
		}, 10*time.Second, 1*time.Second).Should(Equal("ok"))
	})

	Context("configuration", func() {
		Specify("the gateway should be configured correctly", func() {
			docs, err := mgmtClient.GetConfig(context.Background(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())

			objList, err := config.LoadObjects(docs.YAMLDocuments())
			Expect(err).NotTo(HaveOccurred())

			foundConfig := objList.Visit(func(cfg *v1beta1.GatewayConfig) {
				Expect(cfg.Spec.Hostname).To(Equal(outputs.GatewayURL))
			})
			Expect(foundConfig).To(BeTrue())

			foundAuth := objList.Visit(func(ap *v1beta1.AuthProvider) {
				Expect(ap.Spec.Type).To(BeEquivalentTo("openid"))
				openidConf, err := util.DecodeStruct[openid.OpenidConfig](ap.Spec.Options)
				Expect(err).NotTo(HaveOccurred())
				Expect(openidConf.IdentifyingClaim).To(Equal("email"))
				Expect(openidConf.Discovery).NotTo(BeNil())
				Expect(openidConf.Discovery.Issuer).To(Equal(outputs.OAuthIssuerURL))
			})
			Expect(foundAuth).To(BeTrue())
		})

		Specify("grafana should be configured correctly", func() {
			var grafanaConfig k8scorev1.ConfigMap

			err := k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      "grafana-config",
				Namespace: "opni",
			}, &grafanaConfig)

			Expect(err).NotTo(HaveOccurred())
			data := grafanaConfig.Data["grafana.ini"]

			f, err := ini.Load([]byte(data))
			Expect(err).NotTo(HaveOccurred())

			genericOauth, err := f.GetSection("auth.generic_oauth")
			Expect(err).NotTo(HaveOccurred())

			wkc, err := (&openid.OpenidConfig{
				Discovery: &openid.DiscoverySpec{
					Issuer: outputs.OAuthIssuerURL,
				},
			}).GetWellKnownConfiguration()
			Expect(err).NotTo(HaveOccurred())

			kh := genericOauth.KeysHash()
			Expect(kh).To(HaveKeyWithValue("auth_url", wkc.AuthEndpoint))
			Expect(kh).To(HaveKeyWithValue("token_url", wkc.TokenEndpoint))
			Expect(kh).To(HaveKeyWithValue("api_url", wkc.UserinfoEndpoint))
			Expect(kh).To(HaveKeyWithValue("client_id", outputs.OAuthClientID))
			Expect(kh).To(HaveKeyWithValue("client_secret", outputs.OAuthClientSecret))
			Expect(kh).To(HaveKeyWithValue("enabled", "true"))
			Expect(strings.Fields(kh["scopes"])).To(ContainElements("openid", "profile", "email"))

			// This is a very important test: role_attribute_path is a JMESPath expression
			// used to extract a user's grafana role (Viewer, Editor, or Admin) from their
			// ID token. JMESPath has several characters that need to be escaped, one
			// of which is ':' (colon). Many identity providers require custom user
			// attributes to be prefixed with some string such as 'custom:' (cognito)
			// or a URL with a scheme (auth0), both of which contain ':' characters.
			// Unfortunately the only way to do escape sequences in JMESPath is to
			// surround the expression with *double* quotes. This presents a small
			// challenge since the user must configure a string such that when converted
			// from yaml to ini and parsed by the go ini library, the resulting string
			// contains proper double quote characters. If we can parse the grafana
			// ini config using the same library and end up with the correct expression,
			// then we know that the string set in the initial gateway config is correct.
			Expect(kh).To(HaveKeyWithValue("role_attribute_path", `"custom:grafana_role"`))

			server, err := f.GetSection("server")
			Expect(err).NotTo(HaveOccurred())

			kh = server.KeysHash()
			grafanaHostname, err := url.Parse(outputs.GrafanaURL)
			Expect(err).NotTo(HaveOccurred())
			Expect(kh).To(HaveKeyWithValue("domain", grafanaHostname.Host))
			Expect(kh).To(HaveKeyWithValue("root_url", outputs.GrafanaURL))
		})

	})
})
