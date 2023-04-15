//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/openid"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/protobuf/types/known/emptypb"
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
		_, err := mgmtClient.GetCluster(context.Background(), mainClusterRef)
		Expect(err).NotTo(HaveOccurred())
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
		}, 5*time.Minute, 1*time.Second).Should(Equal("ok"))
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
	})
})
