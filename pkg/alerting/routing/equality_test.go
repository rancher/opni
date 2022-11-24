package routing_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/routing"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/samber/lo"
)

const innerHookServer = "http://localhost:3000"

func Create2RoutingTrees() (*routing.RoutingTree, *routing.RoutingTree,
	*routing.OpniInternalRouting, *routing.OpniInternalRouting) {
	return routing.NewDefaultRoutingTree(innerHookServer),
		routing.NewDefaultRoutingTree(innerHookServer),
		routing.NewDefaultOpniInternalRouting(),
		routing.NewDefaultOpniInternalRouting()
}

var _ = Describe("Internal routing tests", Ordered, Label(test.Unit, test.Slow), func() {
	When("We maintain opni's internal routing information", func() {
		It("should compare equality between slack receivers", func() {
			testConditionId1 := uuid.New().String()
			testConditionId2 := uuid.New().String()
			testConditionId3 := testConditionId1
			testConditionId4 := testConditionId2
			testEndpointId1 := uuid.New().String()
			testEndpointId2 := uuid.New().String()
			testEndpointId3 := testEndpointId1
			testEndpointId4 := testEndpointId2
			slackConfig1 := alertingv1.SlackEndpoint{
				Channel:    "#channel1",
				WebhookUrl: "http://testWebhook1",
			}
			slackConfig2 := alertingv1.SlackEndpoint{
				Channel:    "#channel2",
				WebhookUrl: "http://testWebhook2",
			}
			r1, r2, ir1, ir2 := Create2RoutingTrees()
			r3, r4, ir3, ir4 := Create2RoutingTrees()
			Expect(r1).NotTo(BeNil())
			Expect(r2).NotTo(BeNil())
			err1 := r1.CreateRoutingNodeForCondition(testConditionId1, &alertingv1.FullAttachedEndpoints{
				Items: []*alertingv1.FullAttachedEndpoint{
					{
						EndpointId: testEndpointId1,
						AlertEndpoint: &alertingv1.AlertEndpoint{
							Name:        "testEndpoint1",
							Description: "testEndpoint1",
							Endpoint: &alertingv1.AlertEndpoint_Slack{
								Slack: &slackConfig1,
							},
						},
						Details: &alertingv1.EndpointImplementation{
							Title: "title1",
							Body:  "body1",
						},
					},
				},
				Details: &alertingv1.EndpointImplementation{
					Title: "title1",
					Body:  "body1",
				},
			}, ir1)
			Expect(err1).To(Succeed())
			err2 := r2.CreateRoutingNodeForCondition(testConditionId2, &alertingv1.FullAttachedEndpoints{
				Items: []*alertingv1.FullAttachedEndpoint{
					{
						EndpointId: testEndpointId1,
						AlertEndpoint: &alertingv1.AlertEndpoint{
							Name:        "testEndpoint2",
							Description: "testEndpoint2",
							Endpoint: &alertingv1.AlertEndpoint_Slack{
								Slack: &slackConfig2,
							},
						},
						Details: &alertingv1.EndpointImplementation{
							Title: "title2",
							Body:  "body2",
						},
					},
				},
				Details: &alertingv1.EndpointImplementation{
					Title: "title2",
					Body:  "body2",
				},
			}, ir2)
			Expect(err2).To(Succeed())
			err3 := r3.CreateRoutingNodeForCondition(testConditionId3, &alertingv1.FullAttachedEndpoints{
				Items: []*alertingv1.FullAttachedEndpoint{
					{
						EndpointId: testEndpointId3,
						AlertEndpoint: &alertingv1.AlertEndpoint{
							Name:        "testEndpoint1",
							Description: "testEndpoint1",
							Endpoint: &alertingv1.AlertEndpoint_Slack{
								Slack: &slackConfig1,
							},
						},
						Details: &alertingv1.EndpointImplementation{
							Title: "title1",
							Body:  "body1",
						},
					},
				},
				Details: &alertingv1.EndpointImplementation{
					Title: "title1",
					Body:  "body1",
				},
			}, ir3)
			Expect(err3).To(Succeed())
			err4 := r4.CreateRoutingNodeForCondition(testConditionId4, &alertingv1.FullAttachedEndpoints{
				Items: []*alertingv1.FullAttachedEndpoint{
					{
						EndpointId: testEndpointId4,
						AlertEndpoint: &alertingv1.AlertEndpoint{
							Name:        "testEndpoint2",
							Description: "testEndpoint2",
							Endpoint: &alertingv1.AlertEndpoint_Slack{
								Slack: &slackConfig2,
							},
						},
						Details: &alertingv1.EndpointImplementation{
							Title: "title2",
							Body:  "body2",
						},
					},
				},
				Details: &alertingv1.EndpointImplementation{
					Title: "title2",
					Body:  "body2",
				},
			}, ir4)
			Expect(err4).To(Succeed())
			equal1, _ := r1.IsEqual(r2)
			Expect(equal1).To(BeFalse())
			equal2, _ := r3.IsEqual(r4)
			Expect(equal2).To(BeFalse())
			equal3, _ := r1.IsEqual(r3)
			Expect(equal3).To(BeTrue())
			equal4, _ := r2.IsEqual(r4)
			Expect(equal4).To(BeTrue())

		})

		It("should compare equality between email receivers", func() {
			testConditionId1 := uuid.New().String()
			testConditionId2 := uuid.New().String()
			testConditionId3 := testConditionId1
			testConditionId4 := testConditionId2
			testEndpointId1 := uuid.New().String()
			testEndpointId2 := uuid.New().String()
			testEndpointId3 := testEndpointId1
			testEndpointId4 := testEndpointId2
			emailConfig1 := alertingv1.EmailEndpoint{
				To:               "",
				SmtpFrom:         lo.ToPtr("bot@google.com"),
				SmtpSmartHost:    lo.ToPtr("smtp.google.com:587"),
				SmtpAuthUsername: lo.ToPtr("alex"),
				SmtpAuthPassword: lo.ToPtr("password"),
			}
			emailConfig2 := alertingv1.EmailEndpoint{
				To:               "",
				SmtpFrom:         lo.ToPtr("bot@google.com"),
				SmtpSmartHost:    lo.ToPtr("smtp.google.com:587"),
				SmtpAuthUsername: lo.ToPtr("alex"),
				SmtpAuthPassword: lo.ToPtr("password2"),
			}
			r1, r2, ir1, ir2 := Create2RoutingTrees()
			r3, r4, ir3, ir4 := Create2RoutingTrees()
			Expect(r1).NotTo(BeNil())
			Expect(r2).NotTo(BeNil())
			err1 := r1.CreateRoutingNodeForCondition(testConditionId1, &alertingv1.FullAttachedEndpoints{
				Items: []*alertingv1.FullAttachedEndpoint{
					{
						EndpointId: testEndpointId1,
						AlertEndpoint: &alertingv1.AlertEndpoint{
							Name:        "testEndpoint1",
							Description: "testEndpoint1",
							Endpoint: &alertingv1.AlertEndpoint_Email{
								Email: &emailConfig1,
							},
						},
						Details: &alertingv1.EndpointImplementation{
							Title: "title1",
							Body:  "body1",
						},
					},
				},
				Details: &alertingv1.EndpointImplementation{
					Title: "title1",
					Body:  "body1",
				},
			}, ir1)
			Expect(err1).To(Succeed())
			err2 := r2.CreateRoutingNodeForCondition(testConditionId2, &alertingv1.FullAttachedEndpoints{
				Items: []*alertingv1.FullAttachedEndpoint{
					{
						EndpointId: testEndpointId1,
						AlertEndpoint: &alertingv1.AlertEndpoint{
							Name:        "testEndpoint2",
							Description: "testEndpoint2",
							Endpoint: &alertingv1.AlertEndpoint_Email{
								Email: &emailConfig2,
							},
						},
						Details: &alertingv1.EndpointImplementation{
							Title: "title2",
							Body:  "body2",
						},
					},
				},
				Details: &alertingv1.EndpointImplementation{
					Title: "title2",
					Body:  "body2",
				},
			}, ir2)
			Expect(err2).To(Succeed())
			err3 := r3.CreateRoutingNodeForCondition(testConditionId3, &alertingv1.FullAttachedEndpoints{
				Items: []*alertingv1.FullAttachedEndpoint{
					{
						EndpointId: testEndpointId3,
						AlertEndpoint: &alertingv1.AlertEndpoint{
							Name:        "testEndpoint1",
							Description: "testEndpoint1",
							Endpoint: &alertingv1.AlertEndpoint_Email{
								Email: &emailConfig1,
							},
						},
						Details: &alertingv1.EndpointImplementation{
							Title: "title1",
							Body:  "body1",
						},
					},
				},
				Details: &alertingv1.EndpointImplementation{
					Title: "title1",
					Body:  "body1",
				},
			}, ir3)
			Expect(err3).To(Succeed())
			err4 := r4.CreateRoutingNodeForCondition(testConditionId4, &alertingv1.FullAttachedEndpoints{
				Items: []*alertingv1.FullAttachedEndpoint{
					{
						EndpointId: testEndpointId4,
						AlertEndpoint: &alertingv1.AlertEndpoint{
							Name:        "testEndpoint2",
							Description: "testEndpoint2",
							Endpoint: &alertingv1.AlertEndpoint_Email{
								Email: &emailConfig2,
							},
						},
						Details: &alertingv1.EndpointImplementation{
							Title: "title2",
							Body:  "body2",
						},
					},
				},
				Details: &alertingv1.EndpointImplementation{
					Title: "title2",
					Body:  "body2",
				},
			}, ir4)
			Expect(err4).To(Succeed())
			equal1, _ := r1.IsEqual(r2)
			Expect(equal1).To(BeFalse())
			equal2, _ := r3.IsEqual(r4)
			Expect(equal2).To(BeFalse())
			equal3, _ := r1.IsEqual(r3)
			Expect(equal3).To(BeTrue())
			equal4, _ := r2.IsEqual(r4)
			Expect(equal4).To(BeTrue())
		})

		It("should compare equality between pager duty receivers", func() {
			testConditionId1 := uuid.New().String()
			testConditionId2 := uuid.New().String()
			testConditionId3 := testConditionId1
			testConditionId4 := testConditionId2
			testEndpointId1 := uuid.New().String()
			testEndpointId2 := uuid.New().String()
			testEndpointId3 := testEndpointId1
			testEndpointId4 := testEndpointId2
			pagerDutyConfig1 := alertingv1.PagerDutyEndpoint{
				IntegrationKey: "testIntegrationKey1",
			}
			pagerDutyConfig2 := alertingv1.PagerDutyEndpoint{
				IntegrationKey: "testIntegrationKey2",
			}
			r1, r2, ir1, ir2 := Create2RoutingTrees()
			r3, r4, ir3, ir4 := Create2RoutingTrees()
			Expect(r1).NotTo(BeNil())
			Expect(r2).NotTo(BeNil())
			err1 := r1.CreateRoutingNodeForCondition(testConditionId1, &alertingv1.FullAttachedEndpoints{
				Items: []*alertingv1.FullAttachedEndpoint{
					{
						EndpointId: testEndpointId1,
						AlertEndpoint: &alertingv1.AlertEndpoint{
							Name:        "testEndpoint1",
							Description: "testEndpoint1",
							Endpoint: &alertingv1.AlertEndpoint_PagerDuty{
								PagerDuty: &pagerDutyConfig1,
							},
						},
						Details: &alertingv1.EndpointImplementation{
							Title: "title1",
							Body:  "body1",
						},
					},
				},
				Details: &alertingv1.EndpointImplementation{
					Title: "title1",
					Body:  "body1",
				},
			}, ir1)
			Expect(err1).To(Succeed())
			err2 := r2.CreateRoutingNodeForCondition(testConditionId2, &alertingv1.FullAttachedEndpoints{
				Items: []*alertingv1.FullAttachedEndpoint{
					{
						EndpointId: testEndpointId1,
						AlertEndpoint: &alertingv1.AlertEndpoint{
							Name:        "testEndpoint2",
							Description: "testEndpoint2",
							Endpoint: &alertingv1.AlertEndpoint_PagerDuty{
								PagerDuty: &pagerDutyConfig2,
							},
						},
						Details: &alertingv1.EndpointImplementation{
							Title: "title2",
							Body:  "body2",
						},
					},
				},
				Details: &alertingv1.EndpointImplementation{
					Title: "title2",
					Body:  "body2",
				},
			}, ir2)
			Expect(err2).To(Succeed())
			err3 := r3.CreateRoutingNodeForCondition(testConditionId3, &alertingv1.FullAttachedEndpoints{
				Items: []*alertingv1.FullAttachedEndpoint{
					{
						EndpointId: testEndpointId3,
						AlertEndpoint: &alertingv1.AlertEndpoint{
							Name:        "testEndpoint1",
							Description: "testEndpoint1",
							Endpoint: &alertingv1.AlertEndpoint_PagerDuty{
								PagerDuty: &pagerDutyConfig1,
							},
						},
						Details: &alertingv1.EndpointImplementation{
							Title: "title1",
							Body:  "body1",
						},
					},
				},
				Details: &alertingv1.EndpointImplementation{
					Title: "title1",
					Body:  "body1",
				},
			}, ir3)
			Expect(err3).To(Succeed())
			err4 := r4.CreateRoutingNodeForCondition(testConditionId4, &alertingv1.FullAttachedEndpoints{
				Items: []*alertingv1.FullAttachedEndpoint{
					{
						EndpointId: testEndpointId4,
						AlertEndpoint: &alertingv1.AlertEndpoint{
							Name:        "testEndpoint2",
							Description: "testEndpoint2",
							Endpoint: &alertingv1.AlertEndpoint_PagerDuty{
								PagerDuty: &pagerDutyConfig2,
							},
						},
						Details: &alertingv1.EndpointImplementation{
							Title: "title2",
							Body:  "body2",
						},
					},
				},
				Details: &alertingv1.EndpointImplementation{
					Title: "title2",
					Body:  "body2",
				},
			}, ir4)
			Expect(err4).To(Succeed())
			equal1, _ := r1.IsEqual(r2)
			Expect(equal1).To(BeFalse())
			equal2, _ := r3.IsEqual(r4)
			Expect(equal2).To(BeFalse())
			equal3, _ := r1.IsEqual(r3)
			Expect(equal3).To(BeTrue())
			equal4, _ := r2.IsEqual(r4)
			Expect(equal4).To(BeTrue())
		})
	})
})
