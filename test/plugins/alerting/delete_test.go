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
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ = Describe("Invalidated and clean up suite test", Ordered, Label("integration"), func() {
	var env *test.Environment
	var toDeleteMetrics *alertingv1.ConditionReference
	var toDeleteInternal *alertingv1.ConditionReference
	var listRuleRequest *cortexadmin.ListRulesRequest
	agent1 := "agent1"
	agent2 := "agent2"
	BeforeAll(func() {
		env = &test.Environment{}
		Expect(env).NotTo(BeNil())
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)

		alertopsClient := alertops.NewAlertingAdminClient(env.ManagementClientConn())
		cortexOpsClient := cortexops.NewCortexOpsClient(env.ManagementClientConn())
		alertingCondsClient := alertingv1.NewAlertConditionsClient(env.ManagementClientConn())
		mgmtClient := env.NewManagementClient()
		_, err := alertopsClient.InstallCluster(env.Context(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		err = cortexops.InstallWithPreset(context.Background(), cortexOpsClient)
		Expect(err).NotTo(HaveOccurred())
		certsInfo, err := mgmtClient.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		fingerprint := certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
		Expect(fingerprint).NotTo(BeEmpty())
		token, err := mgmtClient.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(1 * time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())

		for _, agent := range []string{agent1, agent2} {
			_, errC := env.StartAgent(agent, token, []string{fingerprint})
			Eventually(errC, time.Second*5, time.Millisecond*200).Should(Receive(BeNil()))
		}

		Eventually(func() error {
			alertingState, err := alertopsClient.GetClusterStatus(env.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			if alertingState.State != alertops.InstallState_Installed {
				return fmt.Errorf("alerting cluster not yet installed")
			}
			cortexState, err := cortexOpsClient.Status(env.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			if cortexState.InstallState != cortexops.InstallState_Installed {
				return fmt.Errorf("cortex cluster not yet installed")
			}
			_, err = alertingCondsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{})
			if err != nil {
				return fmt.Errorf("alerting conditions server not yet available")
			}
			return nil
		}, time.Second*30, time.Second).Should(Succeed())

		_, err = mgmtClient.InstallCapability(env.Context(), &managementv1.CapabilityInstallRequest{
			Name: "metrics",
			Target: &capabilityv1.InstallRequest{
				Cluster: &corev1.Reference{Id: agent1},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		conditionsClient := env.NewAlertConditionsClient()

		ref, err := conditionsClient.CreateAlertCondition(env.Context(), &alertingv1.AlertCondition{
			Name:        "test",
			Description: "",
			Labels:      []string{},
			Severity:    0,
			AlertType: &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_PrometheusQuery{
					PrometheusQuery: &alertingv1.AlertConditionPrometheusQuery{
						ClusterId: &corev1.Reference{Id: agent1},
						Query:     "sum(up > 0) > 0",
						For:       durationpb.New(time.Second * 1),
					},
				},
			},
			AttachedEndpoints: nil,
			Silence:           &alertingv1.SilenceInfo{},
			LastUpdated:       &timestamppb.Timestamp{},
			Id:                "",
			GoldenSignal:      0,
			OverrideType:      "",
			Metadata:          map[string]string{},
		})
		Expect(err).To(Succeed())
		refDisconnect, err := conditionsClient.CreateAlertCondition(env.Context(), &alertingv1.AlertCondition{
			Name:        "agent disconnect",
			Description: "some disconnect to remove",
			Labels:      []string{},
			Severity:    0,
			AlertType: &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_System{
					System: &alertingv1.AlertConditionSystem{
						ClusterId: &corev1.Reference{
							Id: agent1,
						},
						Timeout: durationpb.New(time.Minute * 10),
					},
				},
			},
			Silence:      &alertingv1.SilenceInfo{},
			LastUpdated:  &timestamppb.Timestamp{},
			Id:           "",
			GoldenSignal: 0,
			OverrideType: "",
			Metadata:     map[string]string{},
			GroupId:      "",
		})
		Expect(err).To(Succeed())
		toDeleteInternal = refDisconnect
		toDeleteMetrics = ref
		listRuleRequest = &cortexadmin.ListRulesRequest{
			ClusterId:      []string{agent1, agent2},
			RuleNameRegexp: fmt.Sprintf(".*%s.*", toDeleteMetrics.GetId()),
		}
	})

	When("we update an alarm of the disconnect type", func() {
		It("should successfully update the group of the alarm", func() {
			reduce := func(in *corev1.ReferenceList) []string {
				return lo.Map(in.GetItems(), func(item *corev1.Reference, _ int) string {
					return item.GetId()
				})
			}
			conditionsClient := env.NewAlertConditionsClient()
			By("verifying the only the default group exists")
			groups, err := conditionsClient.ListAlertConditionGroups(env.Context(), &emptypb.Empty{})
			Expect(err).To(Succeed())
			Expect(reduce(groups)).To(ConsistOf([]string{""}))

			cond, err := conditionsClient.GetAlertCondition(env.Context(), toDeleteInternal)
			Expect(err).To(Succeed())
			cond.GroupId = "new-group"

			_, err = conditionsClient.UpdateAlertCondition(env.Context(), &alertingv1.UpdateAlertConditionRequest{
				Id:          toDeleteInternal,
				UpdateAlert: cond,
			})
			Expect(err).NotTo(HaveOccurred())

			newGroups, err := conditionsClient.ListAlertConditionGroups(env.Context(), &emptypb.Empty{})
			Expect(err).To(Succeed())
			Expect(reduce(newGroups)).To(ConsistOf([]string{"", "new-group"}))

			_, err = conditionsClient.GetAlertCondition(env.Context(), toDeleteInternal)
			Expect(err).To(HaveOccurred())
			st, ok := status.FromError(err)
			if !ok {
				Fail("expected to get a grpc error message")
			}
			By("veriying the condition in the old group storage is not found")
			Expect(st.Code()).To(Equal(codes.NotFound))

			By("veryfing the condition has been move to the new group storage")
			toDeleteInternal.GroupId = "new-group"
			updatedCond, err := conditionsClient.GetAlertCondition(env.Context(), toDeleteInternal)
			Expect(err).To(Succeed())
			Expect(updatedCond.GroupId).To(Equal("new-group"))
		})

	})

	XWhen("we delete an alarm of the metrics type", func() {
		It("should successfully submit the alarm for deletion", func() {
			conditionsClient := env.NewAlertConditionsClient()
			adminClient := cortexadmin.NewCortexAdminClient(env.ManagementClientConn())

			By("verifying the metrics conditions eventually are active successfully")
			Eventually(func() error {
				batchList, err := conditionsClient.ListAlertConditionsWithStatus(env.Context(), &alertingv1.ListStatusRequest{})
				if err != nil {
					return err
				}
				if len(batchList.GetAlertConditions()) == 0 {
					return fmt.Errorf("no metrics conditions found")
				}

				for _, item := range batchList.GetAlertConditions() {
					if item.GetAlertCondition().GetAlertType().GetPrometheusQuery() == nil {
						continue
					}
					state := item.GetStatus().GetState()
					if state != alertingv1.AlertConditionState_Firing && state != alertingv1.AlertConditionState_Ok {
						return fmt.Errorf("expected one of the active states firing or ok, got %s", item.GetStatus().GetState())
					}
				}
				return nil
			}, time.Second*30, time.Millisecond*500).Should(Succeed())

			Eventually(func() error {
				rules, err := adminClient.ListRules(env.Context(), listRuleRequest)
				if err != nil {
					return err
				}
				if len(rules.GetData().GetGroups()) == 0 {
					return fmt.Errorf("expected rules found for the metrics condition")
				}
				return nil
			}).Should(Succeed())

			By("deleting the metrics condition")
			_, err := conditionsClient.DeleteAlertCondition(env.Context(), toDeleteMetrics)
			Expect(err).To(Succeed())

			Eventually(func() error {
				alertStatus, err := conditionsClient.AlertConditionStatus(env.Context(), toDeleteMetrics)
				if err != nil {
					if status, ok := status.FromError(err); ok {
						if status.Code() != codes.NotFound {
							return fmt.Errorf("expected a not found code, got %d", status.Code())
						}
					} else {
						return fmt.Errorf("expected a grpc return status code from status operation")
					}
				}
				if alertStatus.GetState() != alertingv1.AlertConditionState_Deleting {
					return fmt.Errorf("only applicable state is deleting")
				}
				return nil
			}, time.Second).Should(Succeed())
		})

		It("should clean up the dependencies of this alarm", func() {
			adminClient := cortexadmin.NewCortexAdminClient(env.ManagementClientConn())

			Eventually(func() error {
				rules, err := adminClient.ListRules(env.Context(), listRuleRequest)
				if err != nil {
					return err
				}
				if len(rules.GetData().GetGroups()) != 0 {
					return fmt.Errorf("expected no rules found for the metrics condition")
				}
				return nil
			}, time.Second*10).Should(Succeed())
		})

		It("should clean up the alarm configuration itself", func() {
			conditionsClient := env.NewAlertConditionsClient()
			Eventually(func() error {
				_, err := conditionsClient.GetAlertCondition(env.Context(), toDeleteMetrics)
				if err == nil {
					return fmt.Errorf("expected an error to have occured while getting the condition")
				}
				if status, ok := status.FromError(err); ok {
					if status.Code() != codes.NotFound {
						return fmt.Errorf("expected a not found code, got %s", status.Code().String())
					}
				} else {
					return fmt.Errorf("expected a grpc return status code from status operation")
				}
				return nil
			}).Should(Succeed())
		})
	})

	When("We uninstall metrics capability from a cluster that has metrics alarms", func() {
		It("should switch these alarms to the invalidated state", func() {
			mgmtClient := env.NewManagementClient()
			_, err := mgmtClient.UninstallCapability(env.Context(), &managementv1.CapabilityUninstallRequest{
				Name: "metrics",
				Target: &capabilityv1.UninstallRequest{
					Cluster: &corev1.Reference{Id: agent1},
				},
			})
			Expect(err).To(Succeed())
			conditionsClient := env.NewAlertConditionsClient()
			Eventually(func() error {
				batchList, err := conditionsClient.ListAlertConditionsWithStatus(env.Context(), &alertingv1.ListStatusRequest{})
				if err != nil {
					return err
				}
				if len(batchList.GetAlertConditions()) == 0 {
					return fmt.Errorf("no metrics conditions found")
				}
				for _, item := range batchList.GetAlertConditions() {
					if item.AlertCondition.AlertType.GetPrometheusQuery() != nil {
						if item.GetStatus().GetState() != alertingv1.AlertConditionState_Invalidated {
							return fmt.Errorf(
								"expected prometheus query to have invalidated state, got %s",
								item.GetStatus().GetState(),
							)
						}
					}
				}
				return nil
			}, time.Second*10, time.Millisecond*500).Should(Succeed())
			Expect(err).To(Succeed())
		})
	})
	When("we delete an alarm of the internal type", func() {
		It("should successfully submit the alarm for deletion", func() {
			conditionsClient := env.NewAlertConditionsClient()

			_, err := conditionsClient.DeleteAlertCondition(env.Context(), toDeleteInternal)
			Expect(err).To(Succeed())
		})

		It("should clean up the alarm configuration itself", func() {
			conditionsClient := env.NewAlertConditionsClient()
			Eventually(func() error {
				_, err := conditionsClient.GetAlertCondition(env.Context(), toDeleteInternal)
				if err == nil {
					return fmt.Errorf("expected to get an error")
				}
				st, ok := status.FromError(err)
				if !ok {
					return fmt.Errorf("expected to get a grpc error message")
				}
				if st.Code() != codes.NotFound {
					return fmt.Errorf("expected to get not found error code instead got %d : %s", st.Code(), st.Code().String())
				}
				return nil
			}, time.Second*5).Should(Succeed())
		})
	})
})
