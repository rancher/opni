package alerting_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	alertmanagerv2 "github.com/prometheus/alertmanager/api/v2/models"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/message"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/alerting"
	"github.com/rancher/opni/pkg/test/testruntime"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	numServers             = 5
	numNotificationServers = 2
)

func init() {
	testruntime.IfIntegration(func() {
		BuildAlertingClusterIntegrationTests([]*alertops.ClusterConfiguration{
			// {
			// 	NumReplicas: 1,
			// 	ResourceLimits: &alertops.ResourceLimitSpec{
			// 		Cpu:     "100m",
			// 		Memory:  "100Mi",
			// 		Storage: "5Gi",
			// 	},
			// 	ClusterSettleTimeout:    "1m",
			// 	ClusterGossipInterval:   "1m",
			// 	ClusterPushPullInterval: "1m",
			// },
			{
				NumReplicas: 3,
				ResourceLimits: &alertops.ResourceLimitSpec{
					Cpu:     "100m",
					Memory:  "100Mi",
					Storage: "5Gi",
				},
				ClusterSettleTimeout:    "1m",
				ClusterGossipInterval:   "1m",
				ClusterPushPullInterval: "1m",
			},
		},
			func() alertops.AlertingAdminClient {
				return alertops.NewAlertingAdminClient(env.ManagementClientConn())
			},
			func() alertingv1.AlertConditionsClient {
				return env.NewAlertConditionsClient()
			},
			func() alertingv1.AlertEndpointsClient {
				return env.NewAlertEndpointsClient()
			},
			func() alertingv1.AlertNotificationsClient {
				return env.NewAlertNotificationsClient()
			},
			func() managementv1.ManagementClient {
				return env.NewManagementClient()
			},
		)
	})
}

type agentWithContext struct {
	id string
	context.Context
	context.CancelFunc
}

func BuildAlertingClusterIntegrationTests(
	clusterConfigurations []*alertops.ClusterConfiguration,
	alertingAdminConstructor func() alertops.AlertingAdminClient,
	alertingConditionsConstructor func() alertingv1.AlertConditionsClient,
	alertingEndpointsConstructor func() alertingv1.AlertEndpointsClient,
	alertingNotificationsConstructor func() alertingv1.AlertNotificationsClient,
	mgmtClientConstructor func() managementv1.ManagementClient,
) bool {
	return Describe("Alerting Cluster Integration tests", Ordered, Label("integration"), func() {
		var alertClusterClient alertops.AlertingAdminClient
		var alertEndpointsClient alertingv1.AlertEndpointsClient
		var alertConditionsClient alertingv1.AlertConditionsClient
		var alertNotificationsClient alertingv1.AlertNotificationsClient
		var mgmtClient managementv1.ManagementClient
		var numAgents int

		// contains agent id and other useful metadata and functions
		var agents []*agentWithContext
		// physical servers that receive opni alerting notifications
		var servers []*alerting.MockIntegrationWebhookServer
		// physical servers that receive all opni alerting notifications
		var notificationServers []*alerting.MockIntegrationWebhookServer
		// conditionsIds => endopintIds
		expectedRouting := map[string]*alertingv1.ConditionReferenceList{}
		// maps condition ids where agents are disconnect to their webhook ids
		involvedDisconnects := map[string][]string{}

		var httpProxyClient *http.Client
		When("Installing the Alerting Cluster", func() {
			BeforeAll(func() {
				alertClusterClient = alertingAdminConstructor()
				alertEndpointsClient = alertingEndpointsConstructor()
				alertConditionsClient = alertingConditionsConstructor()
				alertNotificationsClient = alertingNotificationsConstructor()
				mgmtClient = mgmtClientConstructor()
				numAgents = 5
				tlsConfig := env.GatewayClientTLSConfig()
				httpProxyClient = &http.Client{
					Transport: &http.Transport{
						DialTLS: func(network, addr string) (net.Conn, error) {
							conn, err := tls.Dial(network, addr, tlsConfig)
							return conn, err
						},
					},
				}
			})

			AfterEach(func() {
				status, err := alertClusterClient.GetClusterStatus(env.Context(), &emptypb.Empty{})
				Expect(err).To(BeNil())
				installed := status.State != alertops.InstallState_NotInstalled && status.State != alertops.InstallState_Uninstalling
				if installed {
					info, err := alertClusterClient.Info(env.Context(), &emptypb.Empty{})
					Expect(err).To(BeNil())
					Expect(info).NotTo(BeNil())
					Expect(info.CurSyncId).NotTo(BeEmpty())
					ts := timestamppb.Now()
					for _, comp := range info.Components {
						Expect(comp.LastHandshake.AsTime()).To(BeTemporally("<", ts.AsTime()))
						if comp.ConnectInfo.GetState() != alertops.SyncState_Synced {
							if comp.ConnectInfo.SyncId != info.CurSyncId {
								Expect(comp.LastHandshake.AsTime()).To(BeTemporally(">", ts.AsTime().Add(15*(-time.Second))), "sync is stuttering")
							}
						}
					}
				}
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
					}, time.Second*5, time.Millisecond*200).Should(Succeed())
				})

				It("should apply the configuration", func() {
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
					}, time.Second*5, time.Millisecond*200).Should(Succeed())

					Eventually(func() error {
						getConf, err := alertClusterClient.GetClusterConfiguration(env.Context(), &emptypb.Empty{})
						if !proto.Equal(getConf, clusterConf) {
							return fmt.Errorf("cluster config not equal : not applied")
						}
						return err
					}, time.Second*5, time.Millisecond*200).Should(Succeed())
				})

				Specify("the alerting plugin components should be running and healthy", func() {
					alertingReadinessProbe := fmt.Sprintf("https://%s/plugin_alerting/ready", env.GatewayConfig().Spec.HTTPListenAddress)
					alertingHealthProbe := fmt.Sprintf("https://%s/plugin_alerting/healthy", env.GatewayConfig().Spec.HTTPListenAddress)

					Eventually(func() error {
						reqReady, err := http.NewRequestWithContext(env.Context(), http.MethodGet, alertingReadinessProbe, nil)
						if err != nil {
							return err
						}
						reqHealthy, err := http.NewRequestWithContext(env.Context(), http.MethodGet, alertingHealthProbe, nil)
						if err != nil {
							return err
						}
						respReady, err := httpProxyClient.Do(reqReady)
						if err != nil {
							return err
						}
						defer respReady.Body.Close()
						if respReady.StatusCode != http.StatusOK {
							return fmt.Errorf("alerting plugin not yet ready %d, %s", respReady.StatusCode, respReady.Status)
						}
						respHealthy, err := httpProxyClient.Do(reqHealthy)
						if err != nil {
							return err
						}
						defer respHealthy.Body.Close()
						if respHealthy.StatusCode != http.StatusOK {
							if err != nil {
								return fmt.Errorf("alerting plugin unhealthy %d, %s", respHealthy.StatusCode, respHealthy.Status)
							}
						}
						return nil
					}, time.Second*30, time.Second).Should(Succeed())
				})

				It("should be able to create some endpoints", func() {
					servers = alerting.CreateWebhookServer(numServers)

					for _, server := range servers {
						ref, err := alertEndpointsClient.CreateAlertEndpoint(env.Context(), server.Endpoint())
						Expect(err).To(Succeed())
						server.EndpointId = ref.Id
					}

					By("verifying they are externally persisted")

					endpList, err := alertEndpointsClient.ListAlertEndpoints(env.Context(), &alertingv1.ListAlertEndpointsRequest{})
					Expect(err).To(Succeed())
					Expect(endpList.Items).To(HaveLen(numServers))

					By("verifying they are reachable")

					for _, endp := range endpList.GetItems() {
						Expect(endp.GetId()).NotTo(BeNil())
						_, err := alertNotificationsClient.TestAlertEndpoint(env.Context(), endp.GetId())
						Expect(err).To(Succeed())
					}
					alertingProxyGET := fmt.Sprintf("https://%s/plugin_alerting/alertmanager/api/v2/alerts/groups", env.GatewayConfig().Spec.HTTPListenAddress)
					req, err := http.NewRequestWithContext(env.Context(), http.MethodGet, alertingProxyGET, nil)
					Expect(err).To(Succeed())

					Eventually(func() error {
						resp, err := httpProxyClient.Do(req)
						if err != nil {
							return err
						}
						defer resp.Body.Close()
						if resp.StatusCode != http.StatusOK {
							return fmt.Errorf("expected proxy to return status OK, instead got : %d, %s", resp.StatusCode, resp.Status)
						}
						res := alertmanagerv2.AlertGroups{}
						if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
							return err
						}
						if len(res) == 0 {
							return fmt.Errorf("expected to get empty alertgroup from alertmanager")
						}

						for _, endp := range endpList.GetItems() {
							endpId := endp.GetId().GetId()
							found := false
							for _, ag := range res {
								for _, alert := range ag.Alerts {
									uuid, ok1 := alert.Labels[message.NotificationPropertyOpniUuid]
									nsUuid, ok2 := alert.Labels[message.TestNamespace]
									if ok1 && ok1 == ok2 && uuid == nsUuid {
										foundMatchingRecv := false
										for _, recv := range alert.Receivers {
											if recv.Name != nil && strings.Contains(*recv.Name, endpId) {
												foundMatchingRecv = true
												break
											}
										}
										if foundMatchingRecv {
											found = true
											break
										}
									}
								}
								if found {
									break
								}
							}
							if !found {
								return fmt.Errorf("could not find alert for endpoints %s, %v", endpId, res)
							}
						}
						return nil
					}, time.Second*10, time.Millisecond*200).Should(Succeed())
				})

				It("should create some default conditions when bootstrapping agents", func() {
					By("expecting to have no initial conditions")
					Eventually(func() error {
						condList, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{})
						if err != nil {
							return err
						}
						if len(condList.Items) != 0 {
							return fmt.Errorf("expected 0 conditions, got %d", len(condList.Items))
						}
						return nil
					}, time.Second*30, time.Millisecond*200).Should(Succeed())

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
						_, errC := env.StartAgent(id, token, []string{fingerprint}, test.WithContext(ctxCa))
						Eventually(errC).Should(Receive(BeNil()))
						agents = append(agents, &agentWithContext{
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
					}, time.Second*5, time.Millisecond*200).Should(Succeed())

					By("verifying that only the default group exists")
					groupList, err := alertConditionsClient.ListAlertConditionGroups(env.Context(), &emptypb.Empty{})
					Expect(err).To(Succeed())
					Expect(groupList.Items).To(HaveLen(1))
					Expect(groupList.Items[0].Id).To(Equal(""))
				})

				It("shoud list conditions by given filters", func() {
					for _, agent := range agents {
						Eventually(func() error {
							filteredByCluster, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{
								Clusters: []string{agent.id},
							})
							if err != nil {
								return err
							}
							if len(filteredByCluster.Items) != 2 {
								return fmt.Errorf("expected 2 conditions, got %d", len(filteredByCluster.Items))
							}
							return nil
						}, time.Second).Should(Succeed())

					}

					By("verifying all the agents conditions are critical")

					filterList, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{
						Severities: []alertingv1.OpniSeverity{
							alertingv1.OpniSeverity_Warning,
							alertingv1.OpniSeverity_Error,
							alertingv1.OpniSeverity_Info,
						},
					})
					Expect(err).To(Succeed())
					Expect(filterList.Items).To(HaveLen(0))

					By("verifying we have an equal number of disconnect and capability unhealthy")

					disconnectList, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{
						AlertTypes: []alertingv1.AlertType{
							alertingv1.AlertType_System,
						},
					})
					Expect(err).To(Succeed())
					disconnectStatusList, err := alertConditionsClient.ListAlertConditionsWithStatus(env.Context(), &alertingv1.ListStatusRequest{
						ItemFilter: &alertingv1.ListAlertConditionRequest{
							AlertTypes: []alertingv1.AlertType{
								alertingv1.AlertType_System,
							},
						},
					})
					Expect(err).To(Succeed())

					capabilityList, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{
						AlertTypes: []alertingv1.AlertType{
							alertingv1.AlertType_DownstreamCapability,
						},
					})
					Expect(err).To(Succeed())

					capabilityStatusList, err := alertConditionsClient.ListAlertConditionsWithStatus(env.Context(), &alertingv1.ListStatusRequest{
						ItemFilter: &alertingv1.ListAlertConditionRequest{
							AlertTypes: []alertingv1.AlertType{
								alertingv1.AlertType_DownstreamCapability,
							},
						},
					})

					Expect(err).To(Succeed())
					Expect(capabilityList.Items).To(HaveLen(len(disconnectList.Items)))
					Expect(capabilityStatusList.GetAlertConditions()).To(HaveLen(len(disconnectStatusList.GetAlertConditions())))
				})

				It("should be able to attach endpoints to conditions", func() {
					By("attaching a sample of random endpoints to default agent conditions")
					condList, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{})
					Expect(err).To(Succeed())
					endpList, err := alertEndpointsClient.ListAlertEndpoints(env.Context(), &alertingv1.ListAlertEndpointsRequest{})
					for _, cond := range condList.Items {
						Expect(err).To(Succeed())
						endps := lo.Map(
							lo.Samples(endpList.Items, 1+rand.Intn(len(endpList.Items)-1)),
							func(a *alertingv1.AlertEndpointWithId, _ int) *alertingv1.AttachedEndpoint {
								return &alertingv1.AttachedEndpoint{
									EndpointId: a.GetId().Id,
								}
							})

						if cond.GetAlertCondition().GetAlertType().GetSystem() != nil {
							for _, endp := range endps {
								if _, ok := expectedRouting[endp.EndpointId]; !ok {
									expectedRouting[endp.EndpointId] = &alertingv1.ConditionReferenceList{
										Items: []*alertingv1.ConditionReference{},
									}
								}
								expectedRouting[endp.EndpointId].Items = append(expectedRouting[endp.EndpointId].Items, cond.GetId())
							}
							cond.AlertCondition.AttachedEndpoints = &alertingv1.AttachedEndpoints{
								Items:              endps,
								InitialDelay:       durationpb.New(time.Second * 1),
								ThrottlingDuration: durationpb.New(time.Second * 1),
								Details: &alertingv1.EndpointImplementation{
									Title: "disconnected agent",
									Body:  "agent %s is disconnected",
								},
							}
							_, err = alertConditionsClient.UpdateAlertCondition(env.Context(), &alertingv1.UpdateAlertConditionRequest{
								Id:          cond.GetId(),
								UpdateAlert: cond.AlertCondition,
							})
							Expect(err).To(Succeed())
						}
					}
					for _, refs := range expectedRouting {
						slices.SortFunc(refs.Items, func(a, b *alertingv1.ConditionReference) bool {
							if a.GroupId != b.GroupId {
								return a.GroupId < b.GroupId
							}
							return a.Id < b.Id
						})
					}

					By("creating some default webhook servers as endpoints")
					notificationServers = alerting.CreateWebhookServer(numNotificationServers)
					for _, server := range notificationServers {
						ref, err := alertEndpointsClient.CreateAlertEndpoint(env.Context(), server.Endpoint())
						Expect(err).To(Succeed())
						server.EndpointId = ref.Id
					}
					endpList, err = alertEndpointsClient.ListAlertEndpoints(env.Context(), &alertingv1.ListAlertEndpointsRequest{})
					Expect(err).To(Succeed())
					Expect(endpList.Items).To(HaveLen(numNotificationServers + numServers))

					By("setting the default servers as default endpoints")
					for _, server := range notificationServers {
						_, err = alertEndpointsClient.ToggleNotifications(env.Context(), &alertingv1.ToggleRequest{
							Id: &corev1.Reference{Id: server.EndpointId},
						})
						Expect(err).To(Succeed())
					}

					By("expecting the conditions to eventually move to the 'OK' state")
					Eventually(func() error {
						for _, cond := range condList.Items {
							status, err := alertConditionsClient.AlertConditionStatus(env.Context(), cond.Id)
							if err != nil {
								return err
							}
							if status.State != alertingv1.AlertConditionState_Ok {
								return fmt.Errorf("condition %s \"%s\" is not OK, instead in state %s, %s", cond.AlertCondition.Id, cond.AlertCondition.Name, status.State.String(), status.Reason)
							}
						}
						return nil
					}, time.Second*30, time.Second).Should(Succeed())
					By("verifying the routing relationships are correctly loaded")
					Eventually(func() int {
						relationships, err := alertNotificationsClient.ListRoutingRelationships(env.Context(), &emptypb.Empty{})
						if err != nil {
							return -1
						}
						return len(relationships.RoutingRelationships)
					}).Should(Equal(len(expectedRouting)))
					relationships, err := alertNotificationsClient.ListRoutingRelationships(env.Context(), &emptypb.Empty{})
					Expect(err).To(Succeed())

					for endpId, rel := range relationships.RoutingRelationships {
						slices.SortFunc(rel.Items, func(a, b *alertingv1.ConditionReference) bool {
							if a.GroupId != b.GroupId {
								return a.GroupId < b.GroupId
							}
							return a.Id < b.Id
						})
						if _, ok := expectedRouting[endpId]; !ok {
							Fail(fmt.Sprintf("Expected a routing relation to exist for endpoint id %s", endpId))
						}
						Expect(len(rel.Items)).To(Equal(len(expectedRouting[endpId].Items)))
						for i := range rel.Items {
							Expect(rel.Items[i].Id).To(Equal(expectedRouting[endpId].Items[i].Id))
							Expect(rel.Items[i].GroupId).To(Equal(expectedRouting[endpId].Items[i].GroupId))
						}
					}
				})

				Specify("agent disconnect alarms should fire when agents are disconnected ", func() {
					// Disconnect a random 3 agents, and verify the servers have the messages
					By("disconnecting a random 3 agents")
					disconnectedIds := []string{}
					toDisconnect := lo.Samples(agents, 3)
					for _, disc := range toDisconnect {
						disc.CancelFunc()
						disconnectedIds = append(disconnectedIds, disc.id)
					}

					condList, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{})
					Expect(err).To(Succeed())
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
					webhooks := lo.Uniq(lo.Flatten(lo.Values(involvedDisconnects)))
					Expect(len(webhooks)).To(BeNumerically(">", 0))

					By("verifying the agents are actually disconnected")
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
					}, 5*time.Second, 200*time.Millisecond).Should(Succeed())

					By("verifying the physical servers have received the disconnect messages")
					Eventually(func() error {
						// servers := servers
						conditionIds := lo.Keys(involvedDisconnects)
						for _, id := range conditionIds {
							status, err := alertConditionsClient.AlertConditionStatus(env.Context(), &alertingv1.ConditionReference{Id: id})
							if err != nil {
								return err
							}
							if status.GetState() != alertingv1.AlertConditionState_Firing {
								return fmt.Errorf("expected alerting condition %s to be firing, got %s", id, status.GetState().String())
							}
						}

						for id := range notInvolvedDisconnects {
							status, err := alertConditionsClient.AlertConditionStatus(env.Context(), &alertingv1.ConditionReference{Id: id})
							if err != nil {
								return err
							}
							if status.GetState() != alertingv1.AlertConditionState_Ok {
								return fmt.Errorf("expected unaffected alerting condition %s to be ok, got %s", id, status.GetState().String())
							}
						}

						alertingProxyGET := fmt.Sprintf("https://%s/plugin_alerting/alertmanager/api/v2/alerts/groups", env.GatewayConfig().Spec.HTTPListenAddress)
						req, err := http.NewRequestWithContext(env.Context(), http.MethodGet, alertingProxyGET, nil)
						Expect(err).To(Succeed())

						condList, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{})
						Expect(err).To(Succeed())

						cList := []*alertingv1.AlertCondition{}
						for _, cond := range condList.Items {
							if slices.Contains(conditionIds, cond.GetAlertCondition().GetId()) {
								cList = append(cList, cond.GetAlertCondition())
							}
						}
						Expect(cList).To(HaveLen(len(conditionIds)))

						resp, err := httpProxyClient.Do(req)
						if err != nil {
							return err
						}
						defer resp.Body.Close()
						if resp.StatusCode != http.StatusOK {
							return fmt.Errorf("expected proxy to return status OK, instead got : %d, %s", resp.StatusCode, resp.Status)
						}
						res := alertmanagerv2.AlertGroups{}
						if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
							return err
						}
						if len(res) == 0 {
							return fmt.Errorf("expected to get empty alertgroup from alertmanager")
						}

						// TODO : perhaps refactor into a helper
						for _, cond := range cList {
							condId := cond.GetId()
							found := false
							attachedEndpoints := cond.GetAttachedEndpoints().GetItems()
							for _, ag := range res {
								for _, alert := range ag.Alerts {
									uuid, ok := alert.Labels[message.NotificationPropertyOpniUuid]
									if ok && uuid == condId {
										foundMatchingRecv := true
										if len(attachedEndpoints) > 0 {
											foundMatchingRecv = false
											for _, recv := range alert.Receivers {
												if recv.Name != nil && strings.Contains(*recv.Name, condId) {
													foundMatchingRecv = true
													break
												}
											}
										}
										if foundMatchingRecv {
											found = true
											break
										}
									}
								}
								if found {
									break
								}
							}
							if !found {
								return fmt.Errorf("could not find alert for condition %s, %v", condId, res)
							}
						}
						return nil
					}, time.Second*60, time.Millisecond*500).Should(Succeed())
				})

				It("should be able to batch list status and filter by status", func() {
					condList, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{})
					Expect(err).To(Succeed())

					statusCondList, err := alertConditionsClient.ListAlertConditionsWithStatus(env.Context(), &alertingv1.ListStatusRequest{})
					Expect(err).To(Succeed())
					Expect(statusCondList.AlertConditions).To(HaveLen(len(condList.Items)))
					for condId, cond := range statusCondList.AlertConditions {
						if slices.Contains(lo.Keys(involvedDisconnects), condId) {
							Expect(cond.Status.State).To(Equal(alertingv1.AlertConditionState_Firing))
						} else {
							Expect(cond.Status.State).To(Equal(alertingv1.AlertConditionState_Ok))
						}
					}
					firingOnlyStatusList, err := alertConditionsClient.ListAlertConditionsWithStatus(env.Context(), &alertingv1.ListStatusRequest{
						States: []alertingv1.AlertConditionState{
							alertingv1.AlertConditionState_Firing,
						},
					})
					Expect(err).To(Succeed())
					Expect(firingOnlyStatusList.AlertConditions).To(HaveLen(len(involvedDisconnects)))
					for _, cond := range firingOnlyStatusList.AlertConditions {
						Expect(cond.Status.State).To(Equal(alertingv1.AlertConditionState_Firing))
					}
				})

				It("should return warnings when trying to edit/delete alert endpoints that are involved in conditions", func() {
					webhooks := lo.Uniq(lo.Flatten(lo.Values(involvedDisconnects)))
					Expect(len(webhooks)).To(BeNumerically(">", 0))

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

				It("should have a functional timeline", func() {
					condList, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{})
					Expect(err).To(Succeed())

					// By("verifying the timeline shows only the firing conditions ")
					Eventually(func() error {
						timeline, err := alertConditionsClient.Timeline(env.Context(), &alertingv1.TimelineRequest{
							LookbackWindow: durationpb.New(time.Minute * 5),
						})
						if err != nil {
							return err
						}

						By("verifying the timeline matches the conditions")
						if len(timeline.Items) != len(condList.Items) {
							return fmt.Errorf("expected timeline to have %d items, got %d", len(condList.Items), len(timeline.Items))
						}

						for id, item := range timeline.GetItems() {
							if slices.Contains(lo.Keys(involvedDisconnects), id) {
								if len(item.Windows) == 0 {
									return fmt.Errorf("firing condition should show up on timeline, but does not")
								}
								if len(item.Windows) != 1 {
									return fmt.Errorf("condition evaluation is flaky, should only have one window, but has %d", len(item.Windows))
								}
								messages, err := alertNotificationsClient.ListAlarmMessages(env.Context(), &alertingv1.ListAlarmMessageRequest{
									ConditionId: &alertingv1.ConditionReference{
										Id: id,
									},
									Fingerprints: item.Windows[0].Fingerprints,
									Start:        item.Windows[0].Start,
									End:          timestamppb.Now(),
								})
								if err != nil {
									return err
								}
								if !(len(messages.Items) > 0) {
									return fmt.Errorf("expected firing condition to have cached messages")
								}
								Expect(len(messages.Items)).To(BeNumerically(">", 0))

							} else {
								if len(item.Windows) != 0 {
									return fmt.Errorf("conditions that have not fired should not show up on timeline, but do")
								}
							}
						}
						return nil
					}, time.Second*15, time.Second).Should(Succeed())
				})

				Specify("the alertmanager proxy served by the Gateway HTTP port should be able to list the alarms", func() {
					alertingProxyGET := fmt.Sprintf("https://%s/plugin_alerting/alertmanager/api/v2/alerts/groups", env.GatewayConfig().Spec.HTTPListenAddress)
					req, err := http.NewRequestWithContext(env.Context(), http.MethodGet, alertingProxyGET, nil)
					Expect(err).To(Succeed())

					Eventually(func() error {
						resp, err := httpProxyClient.Do(req)
						if err != nil {
							return err
						}
						defer resp.Body.Close()
						if resp.StatusCode != http.StatusOK {
							return fmt.Errorf("expected proxy to return status OK, instead got : %d, %s", resp.StatusCode, resp.Status)
						}
						res := alertmanagerv2.AlertGroups{}
						if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
							return err
						}
						if len(res) == 0 {
							return fmt.Errorf("expected to get non-empty alertgroup from alertmanager")
						}
						return nil
					}).Should(Succeed())
				})

				It("should sync friendly cluster names to conditions", func() {
					friendlyName := "friendly-name"
					agentId := agents[0].id
					cl, err := mgmtClient.GetCluster(env.Context(), &corev1.Reference{Id: agentId})
					Expect(err).To(Succeed())

					_, err = mgmtClient.EditCluster(env.Context(), &managementv1.EditClusterRequest{
						Cluster: &corev1.Reference{Id: agentId},
						Labels: lo.Assign(cl.GetLabels(), map[string]string{
							corev1.NameLabel: friendlyName,
						}),
					})
					Expect(err).To(Succeed())

					Eventually(func() error {
						conds, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{
							Clusters: []string{agentId},
						})
						if err != nil {
							return err
						}

						for _, cond := range conds.Items {
							name, ok := cond.GetAlertCondition().Annotations[message.NotificationContentClusterName]
							if !ok {
								return fmt.Errorf("expected alert condition to contain annotations with cluster name")
							}
							if name != friendlyName {
								return fmt.Errorf("expected alert condition to contain annotations with cluster name %s, got %s", friendlyName, name)
							}
						}
						return nil
					}, time.Second*5, time.Millisecond*300).Should(Succeed())

				})

				// TODO : this is flaky in CI
				XIt("should force update/delete alert endpoints involved in conditions", func() {
					By("verifying we can edit Alert Endpoints in use by Alert Conditions")
					endpList, err := alertEndpointsClient.ListAlertEndpoints(env.Context(), &alertingv1.ListAlertEndpointsRequest{})
					Expect(err).NotTo(HaveOccurred())
					Expect(len(endpList.Items)).To(BeNumerically(">", 0))
					Expect(endpList.Items).To(HaveLen(numServers + numNotificationServers))
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
								Id: endp.Id.Id,
							},
							ForceUpdate: true,
						})
						Expect(err).NotTo(HaveOccurred())
					}
					endpList, err = alertEndpointsClient.ListAlertEndpoints(env.Context(), &alertingv1.ListAlertEndpointsRequest{})
					Expect(err).NotTo(HaveOccurred())
					Expect(len(endpList.Items)).To(BeNumerically(">", 0))
					Expect(endpList.Items).To(HaveLen(numServers + numNotificationServers))
					updatedList := lo.Filter(endpList.Items, func(item *alertingv1.AlertEndpointWithId, _ int) bool {
						if item.Endpoint.GetWebhook() != nil {
							return item.Endpoint.GetName() == "update" && item.Endpoint.GetDescription() == "update"
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

					condList, err := alertConditionsClient.ListAlertConditions(env.Context(), &alertingv1.ListAlertConditionRequest{})
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
