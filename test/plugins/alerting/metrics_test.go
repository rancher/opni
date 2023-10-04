package alerting_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	alertmanagerv2 "github.com/prometheus/alertmanager/api/v2/models"
	"github.com/rancher/opni/pkg/alerting/message"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/alerting"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	_ "github.com/rancher/opni/plugins/metrics/test"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ = Describe("metrics and alerting", Ordered, Label("integration"), func() {
	var env *test.Environment
	var httpProxyClient *http.Client
	agents := []string{"agent1", "agent2", "agent3"}
	agentAlertingEndpoints := map[string][]*alerting.MockIntegrationWebhookServer{}
	BeforeAll(func() {
		env = &test.Environment{}
		Expect(env).NotTo(BeNil())
		Expect(env.Start()).To(Succeed())
		tlsConfig := env.GatewayClientTLSConfig()
		httpProxyClient = &http.Client{
			Transport: &http.Transport{
				DialTLS: func(network, addr string) (net.Conn, error) {
					conn, err := tls.Dial(network, addr, tlsConfig)
					return conn, err
				},
			},
		}
		DeferCleanup(env.Stop, "Test Suite Finished")
	})
	When("When we use alerting on metrics", func() {
		It("should setup alertig & metrics clusters", func() {
			alertopsClient := alertops.NewAlertingAdminClient(env.ManagementClientConn())
			cortexOpsClient := cortexops.NewCortexOpsClient(env.ManagementClientConn())
			alertingCondsClient := alertingv1.NewAlertConditionsClient(env.ManagementClientConn())
			mgmtClient := env.NewManagementClient()
			_, err := alertopsClient.InstallCluster(env.Context(), &emptypb.Empty{})
			Expect(err).NotTo(HaveOccurred())
			err = cortexops.InstallWithPreset(env.Context(), cortexOpsClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(cortexops.WaitForReady(env.Context(), cortexOpsClient)).To(Succeed())

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
				Eventually(errC, time.Second*10, time.Millisecond*200).Should(Receive(BeNil()))
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
				if cortexState.InstallState != driverutil.InstallState_Installed {
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
				webhooks := alerting.CreateWebhookServer(2)
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
								For:       durationpb.New(time.Second * 10),
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
			}, time.Second*3, time.Millisecond*200).Should(Succeed())
		})

		Specify("the metrics -> alerting pipeline should be functional", FlakeAttempts(4), func() {
			alertConditionsClient := env.NewAlertConditionsClient()
			mgmtClient := env.NewManagementClient()
			cortexAdminClient := cortexadmin.NewCortexAdminClient(env.ManagementClientConn())
			By("installing the metrics capabilities")
			for _, agent := range agents {
				_, err := mgmtClient.InstallCapability(env.Context(), &managementv1.CapabilityInstallRequest{
					Name: "metrics",
					Target: &capabilityv1.InstallRequest{
						Cluster: &corev1.Reference{Id: agent},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			}

			By("verifying the metrics alerts are loaded properly")
			Eventually(func() error {
				allStatuses, err := alertConditionsClient.ListAlertConditionsWithStatus(env.Context(), &alertingv1.ListStatusRequest{
					States: []alertingv1.AlertConditionState{},
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

				clusters, err := mgmtClient.ListClusters(env.Context(), &managementv1.ListClustersRequest{})
				if err != nil {
					return err
				}
				rules, err := cortexAdminClient.ListRules(env.Context(), &cortexadmin.ListRulesRequest{
					ClusterId: lo.Map(clusters.GetItems(), func(cl *corev1.Cluster, _ int) string {
						return cl.GetId()
					}),
					RuleType:        []string{"alerting"},
					NamespaceRegexp: "opni-alerting",
				})
				if err != nil {
					return err
				}
				By("checking the prometheus alert conditions loaded the rule groups for each agent")
				if len(rules.Data.GetGroups()) != len(agents) {
					return fmt.Errorf("not enough rules found %d, expected %d %v", len(rules.Data.GetGroups()), len(agents), rules.Data)
				}

				loadedClusters := lo.Map(rules.Data.GetGroups(), func(g *cortexadmin.RuleGroup, _ int) string {
					return g.ClusterId
				})
				loadedClusters = lo.Uniq(loadedClusters)
				Expect(loadedClusters).To(ConsistOf(agents))

				By("checking we have the correct amount of prometheus alert conditions loaded")
				if len(allStatuses.GetAlertConditions()) != len(agents) {
					return fmt.Errorf("unexpected amount of alert conditions %d, expected %d : %v", len(statuses.GetAlertConditions()), 3, allStatuses.GetAlertConditions())
				}
				errs := []error{}
				numFiring := 0 // FIXME: it looks like the test_driver metrics agent does not always send metrics
				for _, cond := range statuses.GetAlertConditions() {
					status, err := alertConditionsClient.AlertConditionStatus(env.Context(), &alertingv1.ConditionReference{
						Id:      cond.AlertCondition.Id,
						GroupId: cond.AlertCondition.GroupId,
					})
					if err != nil {
						return err
					}
					state := status.State
					if status.State != alertingv1.AlertConditionState_Firing && status.State != alertingv1.AlertConditionState_Ok {
						errs = append(errs, fmt.Errorf("condition %s is in unexpected state %s", cond.AlertCondition.Name, state.String()))
					}
					if status.State == alertingv1.AlertConditionState_Firing {
						numFiring++
					}
				}
				if numFiring == 0 {
					errs = append(errs, errors.New("no sanity metrics are firing"))
				}
				return errors.Join(errs...)
			}, time.Second*10, time.Millisecond*500).Should(Succeed())

			By("verifying the alerts are routed to the correct endpoints in alertmanager ")

			alertingProxyGET := fmt.Sprintf("https://%s/plugin_alerting/alertmanager/api/v2/alerts/groups", env.GatewayConfig().Spec.HTTPListenAddress)
			req, err := http.NewRequestWithContext(env.Context(), http.MethodGet, alertingProxyGET, nil)
			Expect(err).To(Succeed())

			conditionsToMatch, err := alertConditionsClient.ListAlertConditions(
				env.Context(),
				&alertingv1.ListAlertConditionRequest{
					Clusters:   agents,
					AlertTypes: []alertingv1.AlertType{alertingv1.AlertType_PrometheusQuery},
				},
			)
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

				for _, cond := range conditionsToMatch.GetItems() {
					condId := cond.GetAlertCondition().GetId()
					found := false
					attachedEndpoints := cond.GetAlertCondition().GetAttachedEndpoints().GetItems()
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
			}).Should(Succeed())
		})
	})
})
