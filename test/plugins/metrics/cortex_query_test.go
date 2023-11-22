package metrics_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Cortex query tests", Ordered, Label("integration"), func() {
	var environment *test.Environment
	var adminClient cortexadmin.CortexAdminClient
	agentId := "agent-1"
	userId := "user-1"
	capability := &corev1.CapabilityType{
		Name: wellknown.CapabilityMetrics,
	}
	BeforeAll(func() {
		environment = &test.Environment{}
		Expect(environment.Start()).To(Succeed())
		DeferCleanup(environment.Stop)
		client := environment.NewManagementClient()

		err := environment.BootstrapNewAgent(agentId)
		Expect(err).NotTo(HaveOccurred())

		opsClient := cortexops.NewCortexOpsClient(environment.ManagementClientConn())
		err = cortexops.InstallWithPreset(environment.Context(), opsClient)
		Expect(err).NotTo(HaveOccurred())
		Expect(cortexops.WaitForReady(environment.Context(), opsClient)).To(Succeed())

		mgmtClient := environment.NewManagementClient()
		resp, err := mgmtClient.InstallCapability(context.Background(), &capabilityv1.InstallRequest{
			Capability:     &corev1.Reference{Id: wellknown.CapabilityMetrics},
			Agent:          &corev1.Reference{Id: agentId},
			IgnoreWarnings: true,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status).To(Or(
			Equal(capabilityv1.InstallResponseStatus_Success),
			Equal(capabilityv1.InstallResponseStatus_Warning),
		), resp.Message)

		adminClient = cortexadmin.NewCortexAdminClient(environment.ManagementClientConn())

		_, err = client.CreateBackendRole(context.Background(), &corev1.BackendRole{
			Capability: capability,
			Role: &corev1.Role{
				Id: "role-1",
				Permissions: []*corev1.PermissionItem{
					{
						Type: string(corev1.PermissionTypeCluster),
						Verbs: []*corev1.PermissionVerb{
							corev1.VerbGet(),
						},
						Ids: []string{agentId},
					},
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:       "role-binding-1",
			RoleId:   "role-1",
			Subjects: []string{userId},
			Metadata: &corev1.RoleBindingMetadata{
				Capability: lo.ToPtr(wellknown.CapabilityMetrics),
			},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should be able to query metrics from cortex", func() {
		Eventually(func() error {
			statsList, err := adminClient.AllUserStats(context.Background(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			if len(statsList.Items) == 0 {
				return fmt.Errorf("no user stats")
			}
			if statsList.Items[0].NumSeries == 0 {
				return fmt.Errorf("no series")
			}
			return nil
		}, 15*time.Second, 100*time.Millisecond).Should(Succeed())

		client := http.Client{
			Transport: &http.Transport{
				TLSClientConfig: environment.GatewayClientTLSConfig(),
			},
		}
		gatewayAddr := environment.GatewayConfig().Spec.HTTPListenAddress
		req, err := http.NewRequest("GET", fmt.Sprintf("https://%s/api/prom/api/v1/query?query=up", gatewayAddr), nil)
		Expect(err).NotTo(HaveOccurred())

		req.Header.Set("Authorization", userId)
		resp, err := client.Do(req)

		Expect(err).NotTo(HaveOccurred())
		code := resp.StatusCode
		Expect(code).To(Equal(http.StatusOK))
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(body).NotTo(BeEmpty())
	})
})
