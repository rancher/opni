package alerting_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/testruntime"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("agent capability tests", Ordered, Label("integration"), func() {
	var env *test.Environment
	var agents []string = []string{"agent1", "agent2", "agent3"}
	BeforeAll(func() {
		testruntime.IfIntegration(func() {
			env = &test.Environment{}
			Expect(env).NotTo(BeNil())
			Expect(env.Start()).To(Succeed())
			DeferCleanup(env.Stop)
		})
	})

	When("we use the alerting downstream capability", func() {
		It("should install the alerting cluster", func() {
			alertClusterClient := alertops.NewAlertingAdminClient(env.ManagementClientConn())
			_, err := alertClusterClient.InstallCluster(env.Context(), &emptypb.Empty{})
			Expect(err).To(BeNil())

			Eventually(func() error {
				status, err := alertClusterClient.GetClusterStatus(env.Context(), &emptypb.Empty{})
				if err != nil {
					return err
				}
				if status.State != alertops.InstallState_Installed {
					return fmt.Errorf("alerting cluster install state is %s", status.State.String())
				}
				return nil
			}, time.Second*30, time.Second*5).Should(Succeed())

			alertConditionsClient := env.NewAlertConditionsClient()
			Eventually(func() error { // FIXME: cortex CC take forever to acquire in the alerting plugin
				_, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{})
				return err
			}, time.Second*30, time.Millisecond*500).Should(Succeed())
		})

		It("should be able to install the alerting capability on all downstream agents", func() {
			mgmtClient := env.NewManagementClient()

			By("bootstrapping agents")
			certsInfo, err := mgmtClient.CertsInfo(context.Background(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			fingerprint := certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
			Expect(fingerprint).NotTo(BeEmpty())

			token, err := mgmtClient.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(1 * time.Hour),
			})
			Expect(err).NotTo(HaveOccurred())
			for _, agent := range agents {
				_, errC := env.StartAgent(agent, token, []string{fingerprint})
				Eventually(errC).Should(Receive(BeNil()))
			}

			for _, agent := range agents {
				resp, err := mgmtClient.InstallCapability(context.Background(), &managementv1.CapabilityInstallRequest{
					Name: wellknown.CapabilityAlerting,
					Target: &capabilityv1.InstallRequest{
						Cluster: &corev1.Reference{
							Id: agent,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Status).To(Equal(capabilityv1.InstallResponseStatus_Success))
			}
		})

		It("should have synced downstream prometheus alerting rules", func() {
			alertConditionsClient := env.NewAlertConditionsClient()
			Eventually(func() error {
				for _, agent := range agents {
					conds, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{
						Clusters: []string{agent},
						GroupIds: []string{"test-group"},
					})
					Expect(err).To(Succeed())

					for _, cond := range conds.Items {
						Expect(cond.GetAlertCondition().Metadata).NotTo(BeNil())
						_, ok := cond.GetAlertCondition().Metadata["readOnly"]
						Expect(ok).To(BeTrue())
						Expect(cond.GetAlertCondition().GetAlertType().GetPrometheusQuery()).NotTo(BeNil())
					}
				}
				return nil
			}, time.Second*5, time.Millisecond*400).Should(Succeed())
		})
		It("should keep track of the alerting groups", func() {
			alertConditionsClient := env.NewAlertConditionsClient()
			Eventually(func() error {
				groups, err := alertConditionsClient.ListAlertConditionGroups(env.Context(), &emptypb.Empty{})
				if err != nil {
					return err
				}
				if len(groups.Items) != 2 {
					return fmt.Errorf("expected 2 groups, got %d", len(groups.Items))
				}
				if groups.Items[0].Id != "" {
					return fmt.Errorf("expected first group to be the default group")
				}
				if groups.Items[1].Id != "test-group" {
					return fmt.Errorf("expected second group to be test-group")
				}
				return nil
			}, time.Second*5, time.Millisecond*200)
		})

		It("should be able to manipulate a sub-set of the synced rule configuration", func() {
			alertConditionsClient := env.NewAlertConditionsClient()
			conds, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{
				Clusters: agents,
				GroupIds: []string{"test-group"},
			})
			Expect(err).To(Succeed())

			for _, cond := range conds.Items {
				newCond := util.ProtoClone(cond)
				newCond.AlertCondition.AlertType.GetPrometheusQuery().Query = "gibberish"
				newCond.AlertCondition.Severity = alertingv1.OpniSeverity_Critical
				_, err := alertConditionsClient.UpdateAlertCondition(env.Context(), &alertingv1.UpdateAlertConditionRequest{
					Id:          newCond.Id,
					UpdateAlert: newCond.GetAlertCondition(),
				})
				Expect(err).To(HaveOccurred()) // FIXME: requires HA consistency fixes here, errors when trying to apply prom rule
			}

			newConds, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{
				Clusters: agents,
				GroupIds: []string{"test-group"},
			})

			Expect(err).To(Succeed())
			Expect(len(newConds.Items)).To(Equal(len(conds.Items)))
			for _, newCond := range newConds.Items {
				Expect(newCond.AlertCondition.AlertType.GetPrometheusQuery().Query).NotTo(Equal("gibberish"))
				Expect(newCond.AlertCondition.Severity).To(Equal(alertingv1.OpniSeverity_Critical))
			}
		})

		It("should be able to uninstall the alerting capability on all downstream agents", func() {
			mgmtClient := env.NewManagementClient()
			for _, agent := range agents {
				_, err := mgmtClient.UninstallCapability(context.Background(), &managementv1.CapabilityUninstallRequest{
					Name: wellknown.CapabilityAlerting,
					Target: &capabilityv1.UninstallRequest{
						Cluster: &corev1.Reference{
							Id: agent,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})
})
