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
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Cortex query tests", Ordered, Label("integration"), func() {
	var environment *test.Environment
	var fingerprint string
	var adminClient cortexadmin.CortexAdminClient
	agentId := "agent-1"
	userId := "user-1"
	BeforeAll(func() {
		environment = &test.Environment{}
		Expect(environment.Start()).To(Succeed())
		client := environment.NewManagementClient()

		certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		fingerprint = certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
		Expect(fingerprint).NotTo(BeEmpty())

		token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(1 * time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())

		_, errC := environment.StartAgent(agentId, token, []string{fingerprint})
		Eventually(errC).Should(Receive(BeNil()))
		environment.StartPrometheus(agentId)

		opsClient := cortexops.NewCortexOpsClient(environment.ManagementClientConn())
		_, err = opsClient.Install(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		mgmtClient := environment.NewManagementClient()
		resp, err := mgmtClient.InstallCapability(context.Background(), &managementv1.CapabilityInstallRequest{
			Name: "metrics",
			Target: &capabilityv1.InstallRequest{
				Cluster: &corev1.Reference{
					Id: agentId,
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status).To(Equal(capabilityv1.InstallResponseStatus_Success))

		adminClient = cortexadmin.NewCortexAdminClient(environment.ManagementClientConn())

		_, err = client.CreateRole(context.Background(), &corev1.Role{
			Id:         "role-1",
			ClusterIDs: []string{agentId},
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = client.CreateRoleBinding(context.Background(), &corev1.RoleBinding{
			Id:       "role-binding-1",
			RoleId:   "role-1",
			Subjects: []string{userId},
		})
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(environment.Stop)
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
