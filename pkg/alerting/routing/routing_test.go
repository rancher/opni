package routing_test

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/backend"
	"github.com/rancher/opni/pkg/alerting/routing"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/test"
)

type TestAPI struct {
	E           *alertingv1.FullAttachedEndpoints
	conditionId string
	err         error
}

// yikes!!
var _ = Describe("Full fledged dynamic opni routing tests", Ordered, Label(test.Unit, test.Slow), func() {
	When("we need the test to have the correct imports lol", func() {
		var err error
		Expect(err).To(BeNil())
	})
	var testRoutingTree *routing.RoutingTree
	var internalRoutingTree *routing.OpniInternalRouting

	BeforeEach(func() {
		testRoutingTree = routing.NewDefaultRoutingTree("http://localhost:8080")
		internalRoutingTree = routing.NewDefaultOpniInternalRouting()
	})

	AfterEach(func() {
		bytes, err := testRoutingTree.Marshal()
		Expect(err).To(Succeed())
		validateErr := backend.ValidateIncomingConfig(string(bytes), logger.NewPluginLogger().Named("alerting"))
		if validateErr != nil {
			err := backend.ReconcileInvalidStateLoop(time.Second, testRoutingTree, logger.NewPluginLogger().Named("alerting"))
			Expect(err).To(Succeed())
		}
	})

	When("We manipulate the AlertManager routing tree based on opni user input", func() {
		It("should be able to construct the new routing nodes for a condition", func() {
			from := "bot@google.com"
			inputs := []TestAPI{
				{
					E:           &alertingv1.FullAttachedEndpoints{},
					conditionId: uuid.New().String(),
					err:         fmt.Errorf(""),
				},
				{
					E: &alertingv1.FullAttachedEndpoints{
						Items: []*alertingv1.FullAttachedEndpoint{
							{
								EndpointId: uuid.New().String(),
								AlertEndpoint: &alertingv1.AlertEndpoint{
									Name:        "test1",
									Description: "description body",
								},
								Details: &alertingv1.EndpointImplementation{
									Title: "test1",
									Body:  "alert body",
								},
							},
						},
						Details: &alertingv1.EndpointImplementation{
							Title: "test1",
							Body:  "alert body",
						},
					},
					conditionId: uuid.New().String(),
					err:         fmt.Errorf(""),
				},
				{
					E: &alertingv1.FullAttachedEndpoints{
						Items: []*alertingv1.FullAttachedEndpoint{
							{
								EndpointId: uuid.New().String(),
								AlertEndpoint: &alertingv1.AlertEndpoint{
									Name:        "test1",
									Description: "description body",
									Endpoint: &alertingv1.AlertEndpoint_Slack{
										Slack: &alertingv1.SlackEndpoint{},
									},
								},
							},
						},
						Details: &alertingv1.EndpointImplementation{
							Title: "test1",
							Body:  "alert body",
						},
					},
					conditionId: uuid.New().String(),
					err:         fmt.Errorf(""),
				},
				{
					E: &alertingv1.FullAttachedEndpoints{
						Items: []*alertingv1.FullAttachedEndpoint{
							{
								EndpointId: uuid.New().String(),
								AlertEndpoint: &alertingv1.AlertEndpoint{
									Name:        "test1",
									Description: "description body",
									Endpoint: &alertingv1.AlertEndpoint_Slack{
										Slack: &alertingv1.SlackEndpoint{
											WebhookUrl: "http://localhost:8080",
											Channel:    "#cool",
										},
									},
								},
								Details: &alertingv1.EndpointImplementation{
									Title: "test1",
									Body:  "alert body",
								},
							},
						},
						Details: &alertingv1.EndpointImplementation{
							Title: "test1",
							Body:  "alert body1",
						},
					},
					conditionId: uuid.New().String(),
					err:         nil,
				},
				{
					E: &alertingv1.FullAttachedEndpoints{
						Items: []*alertingv1.FullAttachedEndpoint{
							{
								EndpointId: uuid.New().String(),
								AlertEndpoint: &alertingv1.AlertEndpoint{
									Name:        "test1",
									Description: "description body",
									Endpoint: &alertingv1.AlertEndpoint_Slack{
										Slack: &alertingv1.SlackEndpoint{
											WebhookUrl: "http://localhost:8080",
											Channel:    "#cool2",
										},
									},
								},
								Details: &alertingv1.EndpointImplementation{
									Title: "test2",
									Body:  "alert body2",
								},
							},
							{
								EndpointId: uuid.New().String(),
								AlertEndpoint: &alertingv1.AlertEndpoint{
									Name:        "test3",
									Description: "description body3",
									Endpoint: &alertingv1.AlertEndpoint_Slack{
										Slack: &alertingv1.SlackEndpoint{
											WebhookUrl: "http://localhost:8080",
											Channel:    "#cool3",
										},
									},
								},
								Details: &alertingv1.EndpointImplementation{
									Title: "test3",
									Body:  "alert body3",
								},
							},
							{
								EndpointId: uuid.New().String(),
								AlertEndpoint: &alertingv1.AlertEndpoint{
									Name:        "test4",
									Description: "description body4",
									Endpoint: &alertingv1.AlertEndpoint_Email{
										Email: &alertingv1.EmailEndpoint{
											To:       "alexandre.lamarre@suse.com",
											SmtpFrom: &from,
										},
									},
								},
								Details: &alertingv1.EndpointImplementation{
									Title: "test4",
									Body:  "alert body4",
								},
							},
						},
						Details: &alertingv1.EndpointImplementation{
							Title: "test3",
							Body:  "alert body3",
						},
					},

					conditionId: uuid.New().String(),
					err:         nil,
				},
			}

			for _, testItem := range inputs {
				err := testRoutingTree.CreateRoutingNodeForCondition(testItem.conditionId, testItem.E, internalRoutingTree)
				if testItem.err != nil {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).To(Succeed())
					for conditionId, _ := range internalRoutingTree.Content {
						idxRoute, err := testRoutingTree.FindRoutes(conditionId)
						Expect(err).To(Succeed())
						idxReceiver, err := testRoutingTree.FindReceivers(conditionId)
						Expect(err).To(Succeed())
						matchers := testRoutingTree.GetRoutes()[idxRoute].Matchers
						for _, m := range matchers {
							if m.Name == shared.BackendConditionIdLabel {
								Expect(m.Value).To(Equal(conditionId))
							}
						}
						for _, metadata := range internalRoutingTree.Content[conditionId] {
							switch metadata.EndpointType {
							case routing.SlackEndpointInternalId:
								Expect(len(testRoutingTree.GetReceivers()[idxReceiver].SlackConfigs)).To(BeNumerically(">=", *metadata.Position))
							case routing.EmailEndpointInternalId:
								Expect(len(testRoutingTree.GetReceivers()[idxReceiver].EmailConfigs)).To(BeNumerically(">=", *metadata.Position))
							default:
								Fail("invalid endpoint type")
							}
						}
					}
				}
			}
		})
	})
})
