package alerting_test

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

func init() {
	BuildAlertingClusterIntegrationTests([]*alertops.ClusterConfiguration{
		{
			NumReplicas: 1,
			ResourceLimits: &alertops.ResourceLimitSpec{
				Cpu:     "100m",
				Memory:  "100Mi",
				Storage: "5Gi",
			},
			ClusterSettleTimeout:    "1m",
			ClusterGossipInterval:   "1m",
			ClusterPushPullInterval: "1m",
		},
		// HA Mode has inconsistent state results
		// {
		// 	NumReplicas: 3,
		// 	ResourceLimits: &alertops.ResourceLimitSpec{
		// 		Cpu:     "100m",
		// 		Memory:  "100Mi",
		// 		Storage: "5Gi",
		// 	},
		// 	ClusterSettleTimeout:    "1m",
		// 	ClusterGossipInterval:   "1m",
		// 	ClusterPushPullInterval: "1m",
		// },
	},
		func() alertops.AlertingAdminClient {
			return env.NewAlertOpsClient()
		},
		func() alertingv1.AlertConditionsClient {
			return env.NewAlertConditionsClient()
		},
		func() alertingv1.AlertEndpointsClient {
			return env.NewAlertEndpointsClient()
		},
		func() alertingv1.AlertTriggersClient {
			return env.NewAlertTriggersClient()
		},
		func() managementv1.ManagementClient {
			return env.NewManagementClient()
		},
	)
}

type agentWithContext struct {
	id string
	context.Context
	context.CancelFunc
	port int
}

func BuildAlertingClusterIntegrationTests(
	clusterConfigurations []*alertops.ClusterConfiguration,
	alertingAdminConstructor func() alertops.AlertingAdminClient,
	alertingConditionsConstructor func() alertingv1.AlertConditionsClient,
	alertingEndpointsConstructor func() alertingv1.AlertEndpointsClient,
	alertingTriggersConstructor func() alertingv1.AlertTriggersClient,
	mgmtClientConstructor func() managementv1.ManagementClient,
) bool {
	return Describe("Alerting Cluster Integration tests", Ordered, func() {
		var alertClusterClient alertops.AlertingAdminClient
		var alertEndpointsClient alertingv1.AlertEndpointsClient
		var alertConditionsClient alertingv1.AlertConditionsClient
		var alertTriggerClient alertingv1.AlertTriggersClient
		var mgmtClient managementv1.ManagementClient
		var numAgents int
		var agents []*agentWithContext
		var servers []*test.MockIntegrationWebhookServer
		When("Installing the Alerting Cluster", func() {
			BeforeAll(func() {
				alertClusterClient = alertingAdminConstructor()
				alertEndpointsClient = alertingEndpointsConstructor()
				alertConditionsClient = alertingConditionsConstructor()
				alertTriggerClient = alertingTriggersConstructor()
				mgmtClient = mgmtClientConstructor()
				numAgents = 5
			})

			for _, clusterConf := range clusterConfigurations {
				It("should install the alerting cluster", func() {
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
				})

				It("should apply the configuration configuration", func() {
					_, err := alertClusterClient.ConfigureCluster(env.Context(), clusterConf)
					if err != nil {
						if s, ok := status.FromError(err); ok { // conflict is ok if using default config
							Expect(s.Code()).To(Equal(codes.FailedPrecondition))
						}
					}
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

					Eventually(func() error {
						getConf, err := alertClusterClient.GetClusterConfiguration(env.Context(), &emptypb.Empty{})
						if !proto.Equal(getConf, clusterConf) {
							return fmt.Errorf("cluster config not equal : not applied")
						}
						return err
					}, time.Minute*30, time.Second*5)
				})

				It("should be able to create some endpoints", func() {
					numServers := numAgents
					servers = env.CreateWebhookServer(env.Context(), numServers)
					for _, server := range servers {
						ref, err := alertEndpointsClient.CreateAlertEndpoint(env.Context(), server.Endpoint())
						Expect(err).To(Succeed())
						server.EndpointId = ref.Id
					}
					endpList, err := alertEndpointsClient.ListAlertEndpoints(env.Context(), &alertingv1.ListAlertEndpointsRequest{})
					Expect(err).To(Succeed())
					Expect(endpList.Items).To(HaveLen(numServers))
				})

				It("should be able to create some conditions with endpoints", func() {
					By("expecting to have no initial conditions")
					condList, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{})
					Expect(err).To(Succeed())
					Expect(condList.Items).To(HaveLen(0))

					By(fmt.Sprintf("bootstrapping %d agents", numAgents))
					certsInfo, err := mgmtClient.CertsInfo(context.Background(), &emptypb.Empty{})
					Expect(err).NotTo(HaveOccurred())
					fingerprint := certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
					Expect(fingerprint).NotTo(BeEmpty())

					token, err := mgmtClient.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
						Ttl: durationpb.New(1 * time.Hour),
					})
					Expect(err).NotTo(HaveOccurred())
					agentIdFunc := func(i int) string {
						return fmt.Sprintf("agent-%d-%s", i, uuid.New().String())
					}
					agents = []*agentWithContext{}
					for i := 0; i < numAgents; i++ {
						ctxCa, ca := context.WithCancel(env.Context())
						id := agentIdFunc(i)
						port, _ := env.StartAgent(id, token, []string{fingerprint}, test.WithContext(ctxCa))
						agents = append(agents, &agentWithContext{
							port:       port,
							CancelFunc: ca,
							Context:    ctxCa,
							id:         id,
						})
					}
					By("verifying that there are default conditions")
					Eventually(func() error {
						condList, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{})
						if err != nil {
							return err
						}
						if len(condList.Items) != numAgents*2 {
							return fmt.Errorf("expected %d conditions, got %d", numAgents*2, len(condList.Items))
						}
						return nil
					}, time.Second*30, time.Second*5).Should(Succeed())

					By("attaching a sample of random endpoints to default agent conditions")

					condList, err = alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{})
					expectedRouting := map[string][]string{}
					Expect(err).To(Succeed())
					for _, cond := range condList.Items {
						endpList, err := alertEndpointsClient.ListAlertEndpoints(env.Context(), &alertingv1.ListAlertEndpointsRequest{})
						Expect(err).To(Succeed())
						endps := lo.Map(
							lo.Samples(endpList.Items, 1+rand.Intn(len(endpList.Items)-1)),
							func(a *alertingv1.AlertEndpointWithId, _ int) *alertingv1.AttachedEndpoint {
								return &alertingv1.AttachedEndpoint{
									EndpointId: a.GetId().Id,
								}
							})
						if cond.GetAlertCondition().GetAlertType().GetSystem() != nil {
							expectedRouting[cond.GetId().Id] = lo.Map(endps, func(a *alertingv1.AttachedEndpoint, _ int) string {
								return a.EndpointId
							})
							cond.AlertCondition.AttachedEndpoints = &alertingv1.AttachedEndpoints{
								Items:              endps,
								InitialDelay:       durationpb.New(time.Second * 1),
								ThrottlingDuration: durationpb.New(time.Second * 1),
								Details: &alertingv1.EndpointImplementation{
									Title: "disconnected agent",
									Body:  "agent %s is disconnected",
								},
							}
							cond.AlertCondition.AlertType.GetSystem().Timeout = durationpb.New(time.Second * 30)
							_, err = alertConditionsClient.UpdateAlertCondition(env.Context(), &alertingv1.UpdateAlertConditionRequest{
								Id:          cond.GetId(),
								UpdateAlert: cond.AlertCondition,
							})
							Expect(err).To(Succeed())
						}
					}

					By("expecting the conditions to eventually move to the 'OK' state")

					Eventually(func() error {
						for _, cond := range condList.Items {
							status, err := alertConditionsClient.AlertConditionStatus(env.Context(), cond.Id)
							if err != nil {
								return err
							}
							if status.State != alertingv1.AlertConditionState_Ok {
								return fmt.Errorf("condition %s is not OK, instead in state %s", cond.AlertCondition.Name, status.State.String())
							}
						}
						return nil
					}, time.Second*90, time.Second*20).Should(Succeed())

					// Disconnect a random 3 agents, and verify the servers have the messages
					By("disconnecting a random 3 agents")
					disconnectedIds := []string{}
					toDisconnect := lo.Samples(agents, 3)
					for _, disc := range toDisconnect {
						disc.CancelFunc()
						disconnectedIds = append(disconnectedIds, disc.id)
					}

					condList, err = alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{})
					Expect(err).To(Succeed())
					involvedDisconnects := map[string][]string{}
					notInvolvedDisconnects := map[string]struct{}{}
					for _, cond := range condList.Items {
						if cond.GetAlertCondition().GetAlertType().GetSystem() != nil {
							if slices.Contains(disconnectedIds, cond.GetAlertCondition().GetAlertType().GetSystem().ClusterId.Id) {
								endps := lo.Map(cond.AlertCondition.AttachedEndpoints.Items,
									func(a *alertingv1.AttachedEndpoint, _ int) string {
										return a.EndpointId
									})
								involvedDisconnects[cond.GetAlertCondition().Id] = endps
							} else {
								notInvolvedDisconnects[cond.GetAlertCondition().Id] = struct{}{}
							}
						}
					}
					By("verifying the alerting plugin's routing relationships match the expected ones")

					relationships, err := alertTriggerClient.ListRoutingRelationships(env.Context(), &emptypb.Empty{})
					Expect(err).To(Succeed())
					Expect(len(relationships.RoutingRelationships)).To(Equal(len(expectedRouting)))
					for conditionId, rel := range relationships.RoutingRelationships {
						Expect(lo.Map(rel.Items, func(c *corev1.Reference, _ int) string {
							return c.Id
						})).To(ConsistOf(expectedRouting[conditionId]))
					}

					webhooks := lo.Uniq(lo.Flatten(lo.Values(involvedDisconnects)))
					Expect(len(webhooks)).To(BeNumerically(">", 0))

					By("verifying editing/deleting endpoints that are involved in conditions return a warning", func() {
						for _, webhook := range webhooks {
							involvedConditions, err := alertEndpointsClient.UpdateAlertEndpoint(env.Context(), &alertingv1.UpdateAlertEndpointRequest{
								Id: &corev1.Reference{
									Id: webhook,
								},
								UpdateAlert: &alertingv1.AlertEndpoint{
									Name:        "update",
									Description: "update",
									Endpoint: &alertingv1.AlertEndpoint_Webhook{
										Webhook: &alertingv1.WebhookEndpoint{
											Url: "http://example.com",
										},
									},
									Id: "id",
								},
								ForceUpdate: false,
							})
							Expect(err).NotTo(HaveOccurred())
							Expect(involvedConditions.Items).NotTo(HaveLen(0))
							involvedConditions, err = alertEndpointsClient.DeleteAlertEndpoint(env.Context(), &alertingv1.DeleteAlertEndpointRequest{
								Id: &corev1.Reference{
									Id: webhook,
								},
								ForceDelete: false,
							})
							Expect(err).NotTo(HaveOccurred())
							Expect(involvedConditions.Items).NotTo(HaveLen(0))
						}
					})

					Eventually(func() error {
						clusters, err := mgmtClient.ListClusters(env.Context(), &managementv1.ListClustersRequest{})
						if err != nil {
							return err
						}
						for _, cl := range clusters.Items {
							if slices.Contains(disconnectedIds, cl.GetId()) {
								healthStatus, err := mgmtClient.GetClusterHealthStatus(env.Context(), cl.Reference())
								if err != nil {
									return err
								}
								if !healthStatus.Status.Connected == false {
									return fmt.Errorf("expected disconnected health status for cluster %s: %s", cl.GetId(), healthStatus.Status.String())
								}
							}

						}
						return nil
					}, 90*time.Second, 5*time.Second).Should(Succeed())

					Eventually(func() error {
						servers := servers
						conditionIds := lo.Keys(involvedDisconnects)
						for _, id := range conditionIds {
							status, err := alertConditionsClient.AlertConditionStatus(env.Context(), &corev1.Reference{Id: id})
							if err != nil {
								return err
							}
							if status.GetState() != alertingv1.AlertConditionState_Firing {
								return fmt.Errorf("expected alerting condition %s to be firing, got %s", id, status.GetState().String())
							}
						}

						for id := range notInvolvedDisconnects {
							status, err := alertConditionsClient.AlertConditionStatus(env.Context(), &corev1.Reference{Id: id})
							if err != nil {
								return err
							}
							if status.GetState() != alertingv1.AlertConditionState_Ok {
								return fmt.Errorf("expected unaffected alerting condition %s to be ok, got %s", id, status.GetState().String())
							}
						}

						for _, server := range servers {
							if slices.Contains(webhooks, server.EndpointId) {
								// hard to map these excatly without recreating the internal routing logic from the routers
								// since we have dedicated routing integration tests, we can just check that the buffer is not empty
								if len(server.GetBuffer()) == 0 {
									return fmt.Errorf("expected webhook server %s to have messages, got %d", server.EndpointId, len(server.Buffer))
								}
							}
						}
						return nil
					}, time.Minute*2, time.Second*15).Should(Succeed())

					By("verifying the timeline shows only the firing conditions")
					timeline, err := alertConditionsClient.Timeline(env.Context(), &alertingv1.TimelineRequest{
						LookbackWindow: durationpb.New(time.Minute * 5),
					})
					Expect(err).To(Succeed())
					// Expect(len(timeline.GetItems())).To(Equal(len(involvedDisconnects)))
					Expect(timeline.GetItems()).To(HaveLen(len(condList.Items)))
					for id, item := range timeline.GetItems() {
						if slices.Contains(lo.Keys(involvedDisconnects), id) {
							Expect(item.Windows).NotTo(HaveLen(0), "firing condition should show up on timeline, but does not")
						} else {
							Expect(item.Windows).To(HaveLen(0), "conditions that have not fired should not show up on timeline, but do")
						}
					}

					By("verifying we can edit Alert Endpoints in use by Alert Conditions")
					endpList, err := alertEndpointsClient.ListAlertEndpoints(env.Context(), &alertingv1.ListAlertEndpointsRequest{})
					Expect(err).NotTo(HaveOccurred())
					Expect(len(endpList.Items)).To(BeNumerically(">", 0))
					for _, endp := range endpList.Items {
						_, err := alertEndpointsClient.UpdateAlertEndpoint(env.Context(), &alertingv1.UpdateAlertEndpointRequest{
							Id: &corev1.Reference{
								Id: endp.Id.Id,
							},
							UpdateAlert: &alertingv1.AlertEndpoint{
								Name:        "update",
								Description: "update",
								Endpoint: &alertingv1.AlertEndpoint_Webhook{
									Webhook: &alertingv1.WebhookEndpoint{
										Url: "http://example.com",
									},
								},
								Id: "id",
							},
							ForceUpdate: true,
						})
						Expect(err).NotTo(HaveOccurred())
					}
					endpList, err = alertEndpointsClient.ListAlertEndpoints(env.Context(), &alertingv1.ListAlertEndpointsRequest{})
					Expect(err).NotTo(HaveOccurred())
					Expect(endpList.Items).To(HaveLen(numAgents))
					updatedList := lo.Filter(endpList.Items, func(item *alertingv1.AlertEndpointWithId, _ int) bool {
						if item.Endpoint.GetWebhook() != nil {
							return item.Endpoint.GetWebhook().Url == "http://example.com" && item.Endpoint.GetName() == "update" && item.Endpoint.GetDescription() == "update"
						}
						return false
					})
					Expect(updatedList).To(HaveLen(len(endpList.Items)))

					By("verifying we can delete Alert Endpoint in use by Alert Conditions")
					for _, endp := range endpList.Items {
						_, err := alertEndpointsClient.DeleteAlertEndpoint(env.Context(), &alertingv1.DeleteAlertEndpointRequest{
							Id: &corev1.Reference{
								Id: endp.Id.Id,
							},
							ForceDelete: true,
						})
						Expect(err).NotTo(HaveOccurred())
					}
					endpList, err = alertEndpointsClient.ListAlertEndpoints(env.Context(), &alertingv1.ListAlertEndpointsRequest{})
					Expect(err).NotTo(HaveOccurred())
					Expect(endpList.Items).To(HaveLen(0))

					condList, err = alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{})
					Expect(err).NotTo(HaveOccurred())
					Expect(condList.Items).NotTo(HaveLen(0))
					hasEndpoints := lo.Filter(condList.Items, func(item *alertingv1.AlertConditionWithId, _ int) bool {
						if item.AlertCondition.AttachedEndpoints != nil {
							return len(item.AlertCondition.AttachedEndpoints.Items) != 0
						}
						return false
					})
					Expect(hasEndpoints).To(HaveLen(0))
				})

				It("should delete the downstream agents", func() {
					client := env.NewManagementClient()
					agents, err := client.ListClusters(env.Context(), &managementv1.ListClustersRequest{})
					Expect(err).NotTo(HaveOccurred())
					for _, agent := range agents.Items {
						_, err := client.DeleteCluster(env.Context(), agent.Reference())
						Expect(err).NotTo(HaveOccurred())
					}
				})

				It("should uninstall the alerting cluster", func() {
					_, err := alertClusterClient.UninstallCluster(env.Context(), &alertops.UninstallRequest{
						DeleteData: true,
					})
					Expect(err).To(BeNil())
				})
			}
		})
	})
}
