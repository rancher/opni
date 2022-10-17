package conditions_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/alerting/metrics"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/metrics/unmarshal"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/condition"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/util/waitctx"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"

	"github.com/rancher/opni/pkg/test"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var testConditionImplementationReference *alertingv1alpha.AlertConditionWithId
var slackId *corev1.Reference
var emailId *corev1.Reference

var _ = Describe("Alerting Conditions integration tests", Ordered, Label(test.Unit, test.Slow), func() {
	ctx := context.Background()
	When("The alerting condition API is passed invalid input it should be robust", func() {
		XSpecify("Create Alert Condition API should be robust to invalid input", func() {
			fmt.Println("Alert condition tests starting")
			toTestCreateCondition := []InvalidInputs{
				{
					req: &alertingv1alpha.AlertCondition{},
					err: fmt.Errorf("invalid input"),
				},
			}

			for _, invalidInput := range toTestCreateCondition {
				_, err := alertingConditionClient.CreateAlertCondition(ctx, invalidInput.req.(*alertingv1alpha.AlertCondition))
				Expect(err).To(HaveOccurred())
			}

		})

		XSpecify("Get Alert Condition API should be robust to invalid input", func() {
			//TODO

		})

		XSpecify("Update Alert Condition API should be robust to invalid input", func() {
			//TODO
		})

		XSpecify("List Alert Condition API should be robust to invalid input", func() {
			//TODO
		})

		XSpecify("Delete Alert Condition API should be robust to invalid input", func() {
			//TODO
		})
	})

	When("The management server starts", func() {
		XSpecify("The cortex webhook handler should be available for use", func() {
			gatewayTls := env.GatewayTLSConfig()
			cortexHookHandler := env.GetAlertingManagementWebhookEndpoint()
			badRequestCortexPayload := condition.NewSimpleMockAlertManagerPayloadFromAnnotations(map[string]string{
				"alertname": "test",
			})
			invalidCortexPayloadBytes, err := json.Marshal(badRequestCortexPayload)
			Expect(err).To(Succeed())
			status, _, err := test.StandaloneHttpRequest(
				"POST",
				cortexHookHandler,
				invalidCortexPayloadBytes,
				nil,
				gatewayTls)
			Expect(err).To(Succeed())
			Expect(status).To(Equal(400))

			goodCortexPayload := condition.NewSimpleMockAlertManagerPayloadFromAnnotations(map[string]string{
				"conditionId": uuid.New().String(),
				"alertname":   "test",
			})
			goodCortexPayloadBytes, err := json.Marshal(goodCortexPayload)
			Expect(err).To(Succeed())
			status, _, err = test.StandaloneHttpRequest(
				"POST",
				cortexHookHandler,
				goodCortexPayloadBytes,
				nil,
				gatewayTls,
			)
			Expect(err).To(Succeed())
			Expect(status).To(Equal(404))
		})
	})

	When("We mock out backend metrics for alerting", func() {
		XSpecify("We should be able to mock out kubernetes pod metrics", func() {
			fmt.Println("mock pod tests starting")
			Expect(kubernetesTempMetricServerPort).NotTo(Equal(0))
			Expect(curTestState.mockPods).NotTo(BeEmpty())
			// non-deterministically sample mock pods to set
			mp := curTestState.mockPods[rand.Intn(len(curTestState.mockPods))]

			//@debug
			//body := getRawMetrics(kubernetesTempMetricServerPort)
			//bodyBytes, err := ioutil.ReadAll(body)
			//Expect(err).To(Succeed())
			//fmt.Println(bodyBytes)

			for _, state := range metrics.KubeStates {
				Eventually(func() error {
					mp.phase = state
					setMockKubernetesPodState(kubernetesTempMetricServerPort, mp)
					query := fmt.Sprintf(
						"kube_pod_status_phase{pod=\"%s\",namespace=\"%s\",phase=\"%s\", uid=\"%s\"}",
						mp.podName,
						mp.namespace,
						state,
						mp.uid,
					)
					resp, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
						Tenants: []string{"agent"},
						Query:   query,
					})
					if err != nil {
						return err
					}
					q, err := unmarshal.UnmarshalPrometheusResponse(resp.Data)
					if err != nil {
						return err
					}
					var v model.Vector
					switch q.V.Type() {
					case model.ValVector:
						v = q.V.(model.Vector)

					default:
						return fmt.Errorf("cannot unmarshal prometheus response into vector type")
					}
					if len(v) == 0 {
						return fmt.Errorf("no data found")
					}
					for _, sample := range v {
						if sample.Value != 1 {
							return fmt.Errorf("expected 1 sample, got %f", sample.Value)
						}
					}

					return nil
				}, time.Second*10, time.Second).Should(Succeed())
			}
			fmt.Println("mock pods test done")
		})
	})

	When("The alerting plugin starts...", func() {
		XIt("Should be able to CRUD [kubernetes] type alert conditions", func() {
			fmt.Println("cortex kubernetes alert conditions starting")
			alertDuration := durationpb.New(time.Second * 30)
			// non-deterministically sample mock pods to set
			mp := curTestState.mockPods[rand.Intn(len(curTestState.mockPods))]
			resp, err := alertingConditionClient.CreateAlertCondition(ctx, &alertingv1alpha.AlertCondition{
				Name:     "kubernetes-pod-stuck",
				Severity: alertingv1alpha.Severity_WARNING,
				Labels:   []string{mp.uid},
				AlertType: &alertingv1alpha.AlertTypeDetails{
					Type: &alertingv1alpha.AlertTypeDetails_KubeState{
						KubeState: &alertingv1alpha.AlertConditionKubeState{
							ClusterId:  "agent",
							ObjectType: "pod",
							ObjectName: mp.podName,
							Namespace:  mp.namespace,
							State:      mp.phase,
							For:        alertDuration,
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())

			ruleGroupId := alerting.CortexRuleIdFromUuid(resp.Id)
			test.ExpectRuleGroupToExist(
				adminClient,
				ctx,
				"agent",
				ruleGroupId,
				time.Second,
				time.Second*15,
			)

			// need to check the alert condition is actually loaded and evaluated periodically
			Eventually(func() error {
				resp, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
					Tenants: []string{"agent"},
					Query:   fmt.Sprintf("max(ALERTS{alert_name=\"%s\"})", resp.Id),
				})
				if err != nil {
					return err
				}
				q, err := unmarshal.UnmarshalPrometheusResponse(resp.Data)
				if err != nil {
					return err
				}
				qres, err := q.GetVector()
				if err != nil {
					return err
				}
				if len(*qres) == 0 {
					return fmt.Errorf("no data")
				}
				return nil
			}, time.Minute*3, time.Second).Should(Succeed())

			Eventually(func() error {
				resp, err := adminClient.Query(ctx, &cortexadmin.QueryRequest{
					Tenants: []string{"agent"},
					Query:   fmt.Sprintf("max(ALERTS{alert_name=\"%s\"})", resp.Id),
				})
				if err != nil {
					return err
				}
				q, err := unmarshal.UnmarshalPrometheusResponse(resp.Data)
				if err != nil {
					return err
				}
				qres, err := q.GetVector()
				if err != nil {
					return err
				}
				if len(*qres) == 0 {
					return fmt.Errorf("no data")
				}
				return nil
			}, time.Minute*3, time.Second).Should(Succeed())

			// this code block checks that alert has been triggered in opni alerting
			// FIXME: cortex alerting rules are not yet connected to opni alerting
			//Eventually(func() error {
			//	filteredLogs, err := alertingClient.ListAlertLogs(ctx, &alertingv1alpha.ListAlertLogRequest{
			//		Labels: []string{mp.uid},
			//	})
			//	if err != nil {
			//		return err
			//	}
			//	if len(filteredLogs.Items) == 0 {
			//		return fmt.Errorf("no logs found")
			//	}
			//	return nil
			//}, alertDuration.AsDuration(), time.Second*10).Should(Succeed())
			fmt.Println("cortex kubernetes alert conditions finished")
		})

		XIt("Should be able to CRUD [composition] type alert conditions", func() {
			// TODO : when implemented
		})

		XIt("Should be able to CRUD [control flow] type alert conditions", func() {
			// TODO: when implemented
		})

		XIt("Should be CRUD [system] type alert conditions", func() {
			fmt.Println("System Alert Conditions starting")
			conditions, err := alertingConditionClient.ListAlertConditions(ctx, &alertingv1alpha.ListAlertConditionRequest{})
			Expect(err).To(Succeed())
			Expect(conditions.Items).To(HaveLen(1))
			oldCondition := conditions.Items[0].Id

			client := env.NewManagementClient()
			token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Hour),
			})
			Expect(err).NotTo(HaveOccurred())
			info, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
			Expect(err).To(Succeed())

			// on agent startup expect an alert condition to be created
			ctxca, ca := context.WithCancel(waitctx.FromContext(context.Background()))
			p, _ := env.StartAgent("agent", token, []string{info.Chain[len(info.Chain)-1].Fingerprint}, test.WithContext(ctxca))
			Expect(p).NotTo(Equal(0))
			time.Sleep(time.Second)
			newConds, err := alertingConditionClient.ListAlertConditions(ctx, &alertingv1alpha.ListAlertConditionRequest{})
			Expect(err).To(Succeed())
			Expect(newConds.Items).To(HaveLen(2))
			var newConditionId *corev1.Reference
			for _, cond := range newConds.Items {
				if cond.Id != oldCondition {
					newConditionId = cond.Id
				}
			}
			Expect(newConditionId).NotTo(BeNil())

			// kill the agent
			ca()
			Eventually(func() error {
				_, err := alertingLogClient.ListAlertLogs(ctx, &alertingv1alpha.ListAlertLogRequest{})
				if err != nil {
					return err
				}
				//if len(logs.Items) == 0 {
				//	return fmt.Errorf("no logs found, when we should expect them")
				//}
				filteredLogs, err := alertingLogClient.ListAlertLogs(ctx, &alertingv1alpha.ListAlertLogRequest{
					Labels: condition.OpniDisconnect.Labels,
				})
				if err != nil {
					return err
				}
				//if len(logs.Items) == 0 {
				//	return fmt.Errorf("no logs found, when we should expect them")
				//}
				found := false
				for _, log := range filteredLogs.Items {
					if log.ConditionId.Id == newConditionId.Id { // can't use ContainElements matcher because core.Reference eq is quirky
						found = true
						break
					}
				}
				if found {
					return fmt.Errorf("agent condition should not have triggered yet")
				}
				return nil
			}, time.Second*50, time.Second*10).Should(Succeed())

			Eventually(func() error {
				logs, err := alertingLogClient.ListAlertLogs(ctx, &alertingv1alpha.ListAlertLogRequest{})
				if err != nil {
					return err
				}
				if len(logs.Items) == 0 {
					return fmt.Errorf("no logs found, when we should expect them")
				}
				filteredLogs, err := alertingLogClient.ListAlertLogs(ctx, &alertingv1alpha.ListAlertLogRequest{
					Labels: condition.OpniDisconnect.Labels,
				})
				if err != nil {
					return err
				}
				if len(logs.Items) == 0 {
					return fmt.Errorf("no logs found, when we should expect them")
				}
				found := false
				for _, log := range filteredLogs.Items {
					if log.ConditionId.Id == newConditionId.Id { // can't use ContainElements matcher because core.Reference eq is quirky
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("log for the agent disconnected not found")
				}
				return nil
			}, time.Minute*3, time.Second*30).Should(Succeed())
			fmt.Println("System Alert Conditions finished")
		})
	})

	When("We attach notification details to an alert condition", func() {
		// BIG BIG WARNING: we can't actually test if alert manager successfully dispatches the alert to the endpoint
		// BUT we can test that everything is configured correctly & opni alerting processes information correctly
		XIt("should be able to set up a new disconnect condition for testing", func() {
			existing, err := alertingConditionClient.ListAlertConditions(ctx, &alertingv1alpha.ListAlertConditionRequest{})
			Expect(err).To(Succeed())

			client := env.NewManagementClient()
			token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Hour),
			})
			Expect(err).NotTo(HaveOccurred())
			info, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
			Expect(err).To(Succeed())

			// on agent startup expect an alert condition to be created
			ctxca, ca := context.WithCancel(waitctx.FromContext(context.Background()))
			defer ca()
			p, _ := env.StartAgent("agent", token, []string{info.Chain[len(info.Chain)-1].Fingerprint}, test.WithContext(ctxca))
			Expect(p).NotTo(Equal(0))
			time.Sleep(time.Second)

			newConditions, err := alertingConditionClient.ListAlertConditions(ctx, &alertingv1alpha.ListAlertConditionRequest{})
			Expect(err).To(Succeed())
			for _, c := range newConditions.Items {
				for _, c2 := range existing.Items {
					if c2.Id.Id == c.Id.Id {
						testConditionImplementationReference = c
					}
				}
			}
		})

		XIt("Should be able to setup some reusable notification groups", func() {
			existing, err := alertingEndpointClient.ListAlertEndpoints(ctx, &alertingv1alpha.ListAlertEndpointsRequest{})
			Expect(err).To(Succeed())
			// create a notification group

			// create slack
			_, err = alertingEndpointClient.CreateAlertEndpoint(ctx, &alertingv1alpha.AlertEndpoint{
				Name:        "slack",
				Description: "slack endpoint",
				Endpoint: &alertingv1alpha.AlertEndpoint_Slack{
					Slack: &alertingv1alpha.SlackEndpoint{
						WebhookUrl: "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX",
						Channel:    "#opni-alerts",
					},
				},
			})
			Expect(err).To(Succeed())

			newEndpoints, err := alertingEndpointClient.ListAlertEndpoints(ctx, &alertingv1alpha.ListAlertEndpointsRequest{})
			Expect(err).To(Succeed())
			Expect(newEndpoints.Items).To(HaveLen(len(existing.Items) + 1))
			if len(existing.Items) == 0 {
				slackId = newEndpoints.Items[0].Id
			} else {
				found := false
				for _, c := range newEndpoints.Items {
					for _, c2 := range existing.Items {
						if c2.Id.Id == c.Id.Id {
							slackId = c.Id
							found = true
							break
						}
					}
					if found {
						break
					}
				}
			}
			Expect(slackId).NotTo(BeNil())
			existing = newEndpoints

			// create email
			_, err = alertingEndpointClient.CreateAlertEndpoint(ctx, &alertingv1alpha.AlertEndpoint{
				Name:        "slack",
				Description: "slack endpoint",
				Endpoint: &alertingv1alpha.AlertEndpoint_Email{
					Email: &alertingv1alpha.EmailEndpoint{
						To: "alexandre.lamarre@suse.com",
					},
				},
			})
			Expect(err).To(Succeed())
			newEndpoints, err = alertingEndpointClient.ListAlertEndpoints(ctx, &alertingv1alpha.ListAlertEndpointsRequest{})
			Expect(err).To(Succeed())
			for _, c := range newEndpoints.Items {
				found := false
				for _, c2 := range existing.Items {
					if c2.Id.Id == c.Id.Id {
						found = true
						break
					}
				}
				if !found {
					emailId = c.Id
					break
				}
			}
			Expect(emailId).NotTo(BeNil())

		})

		XIt("Should be able to send slack notifications when the condition fails", func() {
			// Expect(testConditionImplementationReference).NotTo(BeNil())
			// Expect(slackId).NotTo(BeNil())
			// _, err := alertingConditionClient.UpdateAlertCondition(ctx, &alertingv1alpha.UpdateAlertConditionRequest{
			// 	Id: testConditionImplementationReference.Id,
			// 	UpdateAlert: &alertingv1alpha.AlertCondition{
			// 		Name:           testConditionImplementationReference.AlertCondition.Name,
			// 		Labels:         testConditionImplementationReference.AlertCondition.Labels,
			// 		Description:    testConditionImplementationReference.AlertCondition.Description,
			// 		Severity:       testConditionImplementationReference.AlertCondition.Severity,
			// 		AlertType:      testConditionImplementationReference.AlertCondition.AlertType,
			// 		NotificationId: &slackId.Id,
			// 		Details: &alertingv1alpha.EndpointImplementation{
			// 			Title: "test",
			// 			Body:  "runtime information about alert",
			// 		},
			// 	},
			// })
			// Expect(err).To(Succeed())
		})

		XIt("Should be able to send email notifications when the condition fails", func() {
			// Expect(testConditionImplementationReference).NotTo(BeNil())
			// e := emailId
			// fmt.Println(e)
			// Expect(emailId).NotTo(BeNil())
			// _, err := alertingConditionClient.UpdateAlertCondition(ctx, &alertingv1alpha.UpdateAlertConditionRequest{
			// 	Id: testConditionImplementationReference.Id,
			// 	UpdateAlert: &alertingv1alpha.AlertCondition{
			// 		Name:           testConditionImplementationReference.AlertCondition.Name,
			// 		Labels:         testConditionImplementationReference.AlertCondition.Labels,
			// 		Description:    testConditionImplementationReference.AlertCondition.Description,
			// 		Severity:       testConditionImplementationReference.AlertCondition.Severity,
			// 		AlertType:      testConditionImplementationReference.AlertCondition.AlertType,
			// 		NotificationId: &emailId.Id,
			// 		Details: &alertingv1alpha.EndpointImplementation{
			// 			Title: "test",
			// 			Body:  "runtime information about alert",
			// 		},
			// 	},
			// })
			// Expect(err).To(Succeed())
		})
	})
})
