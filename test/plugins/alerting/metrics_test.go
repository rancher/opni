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
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/alerting"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	_ "github.com/rancher/opni/plugins/metrics/test"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ = Describe("", Ordered, Label("integration", "alerting"), func() {
	var env *test.Environment
	agents := []string{"agent1", "agent2", "agent3"}
	agentAlertingEndpoints := map[string][]*alerting.MockIntegrationWebhookServer{}
	BeforeAll(func() {
		env = &test.Environment{}
		Expect(env).NotTo(BeNil())
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)
	})
	When("When we use alerting on metrics", func() {
		It("should setup alertig & metrics clusters", func() {
			alertopsClient := alertops.NewAlertingAdminClient(env.ManagementClientConn())
			cortexOpsClient := cortexops.NewCortexOpsClient(env.ManagementClientConn())
			alertingCondsClient := alertingv1.NewAlertConditionsClient(env.ManagementClientConn())
			mgmtClient := env.NewManagementClient()
			_, err := alertopsClient.InstallCluster(env.Context(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			_, err = cortexOpsClient.ConfigureCluster(env.Context(), &cortexops.ClusterConfiguration{
				Mode: cortexops.DeploymentMode_AllInOne,
				Storage: &storagev1.StorageSpec{
					Backend: storagev1.Filesystem,
				},
			})
			Expect(err).NotTo(HaveOccurred())
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

			Eventually(func() error {
				alertingState, err := alertopsClient.GetClusterStatus(env.Context(), &emptypb.Empty{})
				if err != nil {
					return err
				}
				if alertingState.State != alertops.InstallState_Installed {
					return fmt.Errorf("alerting cluster not yet installed")
				}
				cortexState, err := cortexOpsClient.GetClusterStatus(env.Context(), &emptypb.Empty{})
				if err != nil {
					return err
				}
				if cortexState.State != cortexops.InstallState_Installed {
					return fmt.Errorf("cortex cluster not yet installed")
				}
				_, err = alertingCondsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{})
				if err != nil {
					return fmt.Errorf("alerting conditions server not yet available")
				}
				return nil
			}, time.Second*30, time.Second).Should(Succeed())
		})
		It("should create prometheus query alerts that map to endpoints", func() {
			alertEndpointsClient := env.NewAlertEndpointsClient()
			alertConditionsClient := env.NewAlertConditionsClient()
			By("creating webhook endpoints for receiving the prometheus alerting")
			for _, agent := range agents {
				webhooks := alerting.CreateWebhookServer(env, 2)
				for _, webhook := range webhooks {
					ref, err := alertEndpointsClient.CreateAlertEndpoint(env.Context(), webhook.Endpoint())
					Expect(err).To(Succeed())
					webhook.EndpointId = ref.Id
				}
				agentAlertingEndpoints[agent] = webhooks
			}
			By("creating prometheus query alert conditions")
			for _, agent := range agents {
				cond := &alertingv1.AlertCondition{
					Name:        fmt.Sprintf("%s sanity metrics", agent),
					Description: "Fires if there are metrics received from cortex from this agent",
					Labels:      []string{},
					Severity:    0,
					AlertType: &alertingv1.AlertTypeDetails{
						Type: &alertingv1.AlertTypeDetails_PrometheusQuery{
							PrometheusQuery: &alertingv1.AlertConditionPrometheusQuery{
								ClusterId: &corev1.Reference{Id: agent},
								Query:     "sum(up > 0) > 0",
								For:       durationpb.New(time.Second * 2),
							},
						},
					},
					AttachedEndpoints: &alertingv1.AttachedEndpoints{
						Items: lo.Map(agentAlertingEndpoints[agent], func(webhook *alerting.MockIntegrationWebhookServer, _ int) *alertingv1.AttachedEndpoint {
							return &alertingv1.AttachedEndpoint{
								EndpointId: webhook.EndpointId,
							}
						}),
						InitialDelay:       durationpb.New(time.Second * 1),
						RepeatInterval:     durationpb.New(time.Hour * 5),
						ThrottlingDuration: durationpb.New(time.Second * 1),
						Details: &alertingv1.EndpointImplementation{
							Title: "prometheus is receiving metrics",
							Body:  "see title",
						},
					},
					Silence:      &alertingv1.SilenceInfo{},
					LastUpdated:  timestamppb.Now(),
					Id:           "",
					GoldenSignal: 0,
					OverrideType: "",
					Metadata:     map[string]string{},
				}
				_, err := alertConditionsClient.CreateAlertCondition(env.Context(), cond)
				Expect(err).To(Succeed())
			}

			promConds, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{
				Clusters:   agents,
				Severities: []alertingv1.OpniSeverity{},
				Labels:     []string{},
				AlertTypes: []alertingv1.AlertType{
					alertingv1.AlertType_PrometheusQuery,
				},
			})
			Expect(err).To(Succeed())
			Expect(promConds.Items).To(HaveLen(3))
			for _, promCond := range promConds.Items {
				Expect(promCond.GetAlertCondition().GetAlertType().GetPrometheusQuery()).NotTo(BeNil())
			}

			By("making sure when metrics aren't installed, the conditions are invalid")
			Eventually(func() error {
				statuses, err := alertConditionsClient.ListAlertConditionsWithStatus(env.Context(), &alertingv1.ListStatusRequest{
					States: []alertingv1.AlertConditionState{
						alertingv1.AlertConditionState_Invalidated,
					},
					ItemFilter: &alertingv1.ListAlertConditionRequest{
						Clusters:   agents,
						Severities: []alertingv1.OpniSeverity{},
						Labels:     []string{},
						AlertTypes: []alertingv1.AlertType{
							alertingv1.AlertType_PrometheusQuery,
						},
					},
				})
				if err != nil {
					return err
				}
				if len(statuses.GetAlertConditions()) != 3 {
					return fmt.Errorf("unexpected amount of alert conditions %d. expected %d", len(statuses.GetAlertConditions()), 3)
				}
				return nil
			})
		})

		Specify("the metrics -> alerting pipeline should be functional", func() {
			alertConditionsClient := env.NewAlertConditionsClient()
			By("installing the metrics capabilities")
			mgmtClient := env.NewManagementClient()
			for _, agent := range agents {
				_, err := mgmtClient.InstallCapability(env.Context(), &managementv1.CapabilityInstallRequest{
					Name: "metrics",
					Target: &capabilityv1.InstallRequest{
						Cluster: &corev1.Reference{Id: agent},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			}

			By("verifying the conditions move to the firing state", func() {
				By("making sure when metrics aren't installed, the conditions are invalid")
				Eventually(func() error {
					statuses, err := alertConditionsClient.ListAlertConditionsWithStatus(env.Context(), &alertingv1.ListStatusRequest{
						States: []alertingv1.AlertConditionState{
							alertingv1.AlertConditionState_Firing,
						},
						ItemFilter: &alertingv1.ListAlertConditionRequest{
							Clusters:   agents,
							Severities: []alertingv1.OpniSeverity{},
							Labels:     []string{},
							AlertTypes: []alertingv1.AlertType{
								alertingv1.AlertType_PrometheusQuery,
							},
						},
					})
					if err != nil {
						return err
					}
					if len(statuses.GetAlertConditions()) != 3 {
						return fmt.Errorf("unexpected amount of alert conditions %d, expected %d", len(statuses.GetAlertConditions()), 3)
					}
					return nil
				}, time.Second*10, time.Millisecond*500)
			})

			By("verifying the webhook endpoints have received the message")
			Eventually(func() error {
				for agent, webhooks := range agentAlertingEndpoints {
					for _, webhook := range webhooks {
						if len(webhook.GetBuffer()) == 0 {
							return fmt.Errorf("endpoints for conditions created for '%s' agent have not received messages", agent)
						}
					}
				}
				return nil
			}, time.Second*30, time.Millisecond*500).Should(Succeed())
		})
	})
})
