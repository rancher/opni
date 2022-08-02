package alerting_test

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"

	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
)

func curConfig() *alerting.ConfigMapData {
	curConfigData, err := os.ReadFile(alerting.LocalAlertManagerPath)
	Expect(err).To(Succeed())
	curConfig := string(curConfigData)
	configMap := &alerting.ConfigMapData{}
	err = configMap.Parse(curConfig)
	Expect(err).To(Succeed())
	return configMap
}

var idsToCreate = map[string]string{"slack": uuid.New().String(), "email": uuid.New().String()}

var _ = Describe("Alerting Endpoints integration tests", Ordered, Label(test.Unit, test.Slow), func() {
	ctx := context.Background()
	var alertingClient alertingv1alpha.AlertingClient
	// test environment references
	var env *test.Environment
	BeforeAll(func() {

		// setup managemet server & client
		env = &test.Environment{
			TestBin: "../../../testbin/bin",
		}
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)

		// alerting plugin
		alertingClient = alertingv1alpha.NewAlertingClient(env.ManagementClientConn())
	})

	When("The API is passed invalid input, handle it", func() {
		Specify("Create Endpoint API should be robust to invalid input", func() {
			notFromUrl := "not an email url"
			fromUrl := "alexandre.lamarre@suse.com"
			toTestCreateEndpoint := []InvalidInputs{
				{
					req: &alertingv1alpha.AlertEndpoint{},
					err: fmt.Errorf("invalid input"),
				},
				{
					req: &alertingv1alpha.AlertEndpoint{
						Name:        "",
						Description: "",
						Endpoint: &alertingv1alpha.AlertEndpoint_Slack{
							Slack: &alertingv1alpha.SlackEndpoint{
								Name:    "",
								ApiUrl:  "not a url",
								Channel: "#general",
							},
						},
					},
					err: fmt.Errorf("invalid input"),
				},
				{
					req: &alertingv1alpha.AlertEndpoint{
						Name:        "",
						Description: "",
						Endpoint: &alertingv1alpha.AlertEndpoint_Slack{
							Slack: &alertingv1alpha.SlackEndpoint{
								Name:    "",
								ApiUrl:  "http://some.url.com",
								Channel: "not a channel",
							},
						},
					},
					err: fmt.Errorf("invalid input"),
				},
				{
					req: &alertingv1alpha.AlertEndpoint{
						Name:        "",
						Description: "",
						Endpoint: &alertingv1alpha.AlertEndpoint_Email{
							Email: &alertingv1alpha.EmailEndpoint{
								Name: "",
								From: &fromUrl,
								To:   "",
							},
						},
					},
					err: fmt.Errorf("invalid input"),
				},
				{
					req: &alertingv1alpha.AlertEndpoint{
						Name:        "",
						Description: "",
						Endpoint: &alertingv1alpha.AlertEndpoint_Email{
							Email: &alertingv1alpha.EmailEndpoint{
								Name: "",
								From: &fromUrl,
								To:   "asdasdaasdasd",
							},
						},
					},
					err: fmt.Errorf("invalid input"),
				},
				{
					req: &alertingv1alpha.AlertEndpoint{
						Name:        "",
						Description: "",
						Endpoint: &alertingv1alpha.AlertEndpoint_Email{
							Email: &alertingv1alpha.EmailEndpoint{
								Name: "",
								From: &notFromUrl,
								To:   "alexandre.lamarre@suse.com",
							},
						},
					},
					err: fmt.Errorf("invalid input"),
				},
			}

			for _, invalidInput := range toTestCreateEndpoint {
				_, err := alertingClient.CreateAlertEndpoint(ctx, invalidInput.req.(*alertingv1alpha.AlertEndpoint))
				Expect(err).To(HaveOccurred())
			}

		})

		Specify("Get Alert Endpoint API should be robust to invalid input", func() {
			// toTestCreateEndpoint := []InvalidInputs{
			// 	{
			// 		req: &alertingv1alpha.AlertEndpoint{},
			// 		err: fmt.Errorf("invalid input"),
			// 	},
			// }

			// for _, invalidInput := range toTestCreateEndpoint {
			// 	_, err := alertingClient.GetAlertEndpoint(ctx, invalidInput.req.(*corev1.Reference))
			// 	Expect(err).To(HaveOccurred())
			// 	Expect(err.Error()).To(Equal(invalidInput.err.Error()))
			// }

		})

		Specify("Update Alert Endpoint API should be robust to invalid input", func() {
			// toTestCreateEndpoint := []InvalidInputs{
			// 	{
			// 		req: &alertingv1alpha.AlertEndpoint{},
			// 		err: fmt.Errorf("invalid input"),
			// 	},
			// }
			// for _, invalidInput := range toTestCreateEndpoint {
			// 	_, err := alertingClient.UpdateAlertEndpoint(ctx, invalidInput.req.(*alertingv1alpha.UpdateAlertEndpointRequest))
			// 	Expect(err).To(HaveOccurred())
			// 	Expect(err.Error()).To(Equal(invalidInput.err.Error()))
			// }
		})

		Specify("List Alert Endpoint API should be robust to invalid input", func() {
			// toTestCreateEndpoint := []InvalidInputs{
			// 	{
			// 		req: &alertingv1alpha.AlertEndpoint{},
			// 		err: fmt.Errorf("invalid input"),
			// 	},
			// }
			// for _, invalidInput := range toTestCreateEndpoint {
			// 	_, err := alertingClient.ListAlertEndpoints(ctx, invalidInput.req.(*alertingv1alpha.ListAlertEndpointsRequest))
			// 	Expect(err).To(HaveOccurred())
			// 	Expect(err.Error()).To(Equal(invalidInput.err.Error()))
			// }
		})

		Specify("Delete Alert Endpoint API should be robust to invalid input", func() {
			// toTestCreateEndpoint := []InvalidInputs{
			// 	{
			// 		req: &alertingv1alpha.AlertEndpoint{},
			// 		err: fmt.Errorf("invalid input"),
			// 	},
			// }
			// for _, invalidInput := range toTestCreateEndpoint {
			// 	_, err := alertingClient.DeleteAlertEndpoint(ctx, invalidInput.req.(*corev1.Reference))
			// 	Expect(err).To(HaveOccurred())
			// 	Expect(err.Error()).To(Equal(invalidInput.err.Error()))
			// }
		})

		Specify("Test Alert Endpoint API should be robust to invalid input", func() {
			// toTestCreateEndpoint := []InvalidInputs{
			// 	{
			// 		req: &alertingv1alpha.AlertEndpoint{},
			// 		err: shared.AlertingErrNotImplemented,
			// 	},
			// }
			// for _, invalidInput := range toTestCreateEndpoint {
			// 	_, err := alertingClient.TestAlertEndpoint(ctx, invalidInput.req.(*alertingv1alpha.TestAlertEndpointRequest))
			// 	Expect(err).To(HaveOccurred())
			// 	Expect(err.Error()).To(Equal(invalidInput.err.Error()))
			// }
		})

		Specify("Get Implementation From Endpoint API should be robust to invalid input", func() {

		})

		Specify("Create Endpoint Implementation API should be robust to invalid input ", func() {

		})

		Specify("Update Endpoint Implementation API should be robust to invalid input ", func() {

		})

		Specify("Delete Endpoint Implementation API should be robust to invalid input", func() {

		})

		Specify("Cleaning up edge case data", func() {
			err := os.WriteFile(alerting.LocalAlertManagerPath, []byte(alerting.DefaultAlertManager), 0644)
			Expect(err).To(Succeed())
		})

	})

	When("The alerting plugin starts", func() {
		It("Should be able to CRUD (Reusable K,V groups) Alert Endpoints", func() {
			fromUrl := "bot@google.com"
			inputs := []*alertingv1alpha.AlertEndpoint{
				{
					Name:        "TestAlertEndpoint",
					Description: "TestAlertEndpoint",
					Endpoint: &alertingv1alpha.AlertEndpoint_Email{
						Email: &alertingv1alpha.EmailEndpoint{
							Name: "TestAlertEndpoint",
							From: nil,
							To:   "alex7285@gmail.com",
						},
					},
				},
				{
					Name:        "TestAlertEndpoint2",
					Description: "TestAlertEndpoint2",
					Endpoint: &alertingv1alpha.AlertEndpoint_Email{
						Email: &alertingv1alpha.EmailEndpoint{
							Name: "TestAlertEndpoint2",
							To:   "alexandre.lamarre@suse.com",
							From: &fromUrl,
						},
					},
				},
				{
					Name:        "TestAlertEndpoint3",
					Description: "TestAlertEndpoint3",
					Endpoint: &alertingv1alpha.AlertEndpoint_Slack{
						Slack: &alertingv1alpha.SlackEndpoint{
							Name:    "TestAlertEndpoint2",
							Channel: "#channel",
							ApiUrl:  "https://hooks.slack.com/services/T0S0S0S0S/B0S0S0S0S/B0S0S0S0S",
						},
					},
				},
				{
					Name:        "TestAlertEndpoint4",
					Description: "TestAlertEndpoint4",
					Endpoint: &alertingv1alpha.AlertEndpoint_Slack{
						Slack: &alertingv1alpha.SlackEndpoint{
							Name:    "TestAlertEndpoint2",
							Channel: "#general",
							ApiUrl:  "https://hooks.slack.com/services/AAAAAAAA/B0S0S0S0S/B0S0S0S0S",
						},
					},
				},
			}
			for num, input := range inputs {
				_, err := alertingClient.CreateAlertEndpoint(ctx, input)
				Expect(err).To(Succeed())
				existing, err := alertingClient.ListAlertEndpoints(ctx, &alertingv1alpha.ListAlertEndpointsRequest{})
				Expect(err).To(Succeed())
				Expect(existing.Items).To(HaveLen(num + 1))

			}
			alertingClient.CreateAlertEndpoint(ctx, &alertingv1alpha.AlertEndpoint{})
		})

		It("Should be able to update & delete those alert endpoints", func() {
			existing, err := alertingClient.ListAlertEndpoints(ctx, &alertingv1alpha.ListAlertEndpointsRequest{})
			Expect(err).To(Succeed())
			Expect(existing.Items).NotTo(HaveLen(0))
			emailName := "Updated"
			emailDescription := "Udpated"
			emailTo := "alex7285@gmail.com"
			some := existing.Items[0]
			_, err = alertingClient.GetAlertEndpoint(ctx, some.Id)
			Expect(err).To(Succeed())
			_, err = alertingClient.UpdateAlertEndpoint(ctx, &alertingv1alpha.UpdateAlertEndpointRequest{
				Id: some.Id,
				UpdateAlert: &alertingv1alpha.AlertEndpoint{
					Name:        emailName,
					Description: emailDescription,
					Endpoint: &alertingv1alpha.AlertEndpoint_Email{
						Email: &alertingv1alpha.EmailEndpoint{
							Name: emailDescription,
							To:   emailTo,
						},
					},
				},
			})
			Expect(err).To(Succeed())

			newItems, err := alertingClient.ListAlertEndpoints(ctx, &alertingv1alpha.ListAlertEndpointsRequest{})
			Expect(err).To(Succeed())
			Expect(newItems.Items).To(HaveLen(len(existing.Items)))
			var found *alertingv1alpha.AlertEndpointWithId
			for _, item := range newItems.Items {
				if item.Id.Id == some.Id.Id {
					found = item
					break
				}
			}
			Expect(found).NotTo(BeNil())
			Expect(found.Endpoint.Name).To(Equal(emailName))
			Expect(found.Endpoint.Description).To(Equal(emailDescription))
			Expect(found.Endpoint.GetEmail().To).To(Equal(emailTo))

			slackChannel := "#channel"
			slackApiUrl := "https://slack.com/api"

			_, err = alertingClient.UpdateAlertEndpoint(ctx, &alertingv1alpha.UpdateAlertEndpointRequest{
				Id: some.Id,
				UpdateAlert: &alertingv1alpha.AlertEndpoint{
					Name:        emailName,
					Description: emailDescription,
					Endpoint: &alertingv1alpha.AlertEndpoint_Slack{
						Slack: &alertingv1alpha.SlackEndpoint{
							Name:    emailDescription,
							Channel: slackChannel,
							ApiUrl:  slackApiUrl,
						},
					},
				},
			})
			Expect(err).To(Succeed())
			newestItems, err := alertingClient.ListAlertEndpoints(ctx, &alertingv1alpha.ListAlertEndpointsRequest{})
			Expect(err).To(Succeed())
			for _, item := range newestItems.Items {
				if item.Id.Id == some.Id.Id {
					found = item
					break
				}
			}
			Expect(found).NotTo(BeNil())
			Expect(found.Endpoint.Name).To(Equal(emailName))
			Expect(found.Endpoint.Description).To(Equal(emailDescription))
			Expect(found.Endpoint.GetEmail()).To(BeNil())
			Expect(found.Endpoint.GetSlack()).NotTo(BeNil())
			Expect(found.Endpoint.GetSlack().Name).To(Equal(emailDescription))
			Expect(found.Endpoint.GetSlack().Channel).To(Equal(slackChannel))
			Expect(found.Endpoint.GetSlack().ApiUrl).To(Equal(slackApiUrl))

			_, err = alertingClient.DeleteAlertEndpoint(ctx, some.Id)
			Expect(err).To(Succeed())
			newestNewestItems, err := alertingClient.ListAlertEndpoints(ctx, &alertingv1alpha.ListAlertEndpointsRequest{})
			Expect(err).To(Succeed())
			var missing *alertingv1alpha.AlertEndpointWithId
			for _, item := range newestNewestItems.Items {
				if item.Id.Id == some.Id.Id {
					missing = item
					break
				}
			}
			Expect(missing).To(BeNil())
		})

		It("Should be able to get alert endpoint implementation details", func() {
			var slack *alertingv1alpha.AlertEndpointWithId
			var email *alertingv1alpha.AlertEndpointWithId
			existing, err := alertingClient.ListAlertEndpoints(ctx,
				&alertingv1alpha.ListAlertEndpointsRequest{})
			Expect(err).To(Succeed())
			Expect(existing.Items).NotTo(HaveLen(0))
			for _, item := range existing.Items {
				if item.Endpoint.GetSlack() != nil {
					slack = item
				} else if item.Endpoint.GetEmail() != nil {
					email = item
				}
			}
			Expect(slack).NotTo(BeNil())
			Expect(email).NotTo(BeNil())

			s, err := alertingClient.GetImplementationFromEndpoint(ctx, &corev1.Reference{
				Id: slack.Id.Id,
			})
			Expect(err).To(Succeed())
			Expect(s.GetSlack()).NotTo(BeNil())
			sDefaults := (&alertingv1alpha.SlackImplementation{}).Defaults()
			Expect(s.GetSlack()).To(Equal(sDefaults))
			e, err := alertingClient.GetImplementationFromEndpoint(ctx, &corev1.Reference{
				Id: email.Id.Id,
			})
			eDefaults := (&alertingv1alpha.EmailImplementation{}).Defaults()
			Expect(err).To(Succeed())
			Expect(e.GetEmail()).NotTo(BeNil())
			Expect(e.GetEmail()).To(Equal(eDefaults))
		})

		It("Should be able to create endpoint implementations", func() {
			var slack *alertingv1alpha.AlertEndpointWithId
			var email *alertingv1alpha.AlertEndpointWithId
			existing, err := alertingClient.ListAlertEndpoints(ctx,
				&alertingv1alpha.ListAlertEndpointsRequest{})
			Expect(err).To(Succeed())
			Expect(existing.Items).NotTo(HaveLen(0))
			for _, item := range existing.Items {
				if item.Endpoint.GetSlack() != nil {
					slack = item
				} else if item.Endpoint.GetEmail() != nil {
					email = item
				}
			}
			Expect(slack).NotTo(BeNil())
			Expect(email).NotTo(BeNil())

			curConfigData := curConfig()
			Expect(curConfigData.Receivers).NotTo(BeNil())
			Expect(curConfigData.Receivers).To(HaveLen(1))

			Expect(err).To(Succeed())
			Expect(curConfigData.Receivers).To(HaveLen(1))

			_, err = alertingClient.CreateEndpointImplementation(ctx,
				&alertingv1alpha.CreateImplementation{
					NotificationId: slack.Id,
					ConditionId: &corev1.Reference{
						Id: idsToCreate["slack"],
					},
					Implementation: &alertingv1alpha.EndpointImplementation{
						Implementation: &alertingv1alpha.EndpointImplementation_Slack{
							Slack: &alertingv1alpha.SlackImplementation{
								Title:    "Slack title [CI]",
								Text:     "Slack message content [CI]",
								ImageUrl: "Slack image url [CI]",
								Footer:   "information in the footer [CI]",
							},
						},
					},
				},
			)

			Expect(err).To(Succeed())
			Expect(curConfig().Receivers).To(HaveLen(2))

			emailContext := "Email message content [CI]"
			_, err = alertingClient.CreateEndpointImplementation(ctx,
				&alertingv1alpha.CreateImplementation{
					NotificationId: email.Id,
					ConditionId: &corev1.Reference{
						Id: idsToCreate["email"],
					},
					Implementation: &alertingv1alpha.EndpointImplementation{
						Implementation: &alertingv1alpha.EndpointImplementation_Email{
							Email: &alertingv1alpha.EmailImplementation{
								TextBody: &emailContext,
							},
						},
					},
				},
			)
			Expect(err).To(Succeed())
			Expect(curConfig().Receivers).To(HaveLen(3))
		})

		It("Should be able to update endpoint implementations", func() {
			var slack *alertingv1alpha.AlertEndpointWithId
			var email *alertingv1alpha.AlertEndpointWithId
			existing, err := alertingClient.ListAlertEndpoints(ctx,
				&alertingv1alpha.ListAlertEndpointsRequest{})
			Expect(err).To(Succeed())
			Expect(existing.Items).NotTo(HaveLen(0))
			for _, item := range existing.Items {
				if item.Endpoint.GetSlack() != nil {
					slack = item
				} else if item.Endpoint.GetEmail() != nil {
					email = item
				}
			}
			Expect(slack).NotTo(BeNil())
			Expect(email).NotTo(BeNil())
			Expect(curConfig().Receivers).To(HaveLen(3))

			// for an alert condition, update slack to email notification
			newEmailMsg := "Email message content [CI]"
			_, err = alertingClient.UpdateEndpointImplementation(ctx,
				&alertingv1alpha.CreateImplementation{
					NotificationId: email.Id,
					ConditionId: &corev1.Reference{
						Id: idsToCreate["slack"],
					},
					Implementation: &alertingv1alpha.EndpointImplementation{
						Implementation: &alertingv1alpha.EndpointImplementation_Email{
							Email: &alertingv1alpha.EmailImplementation{
								TextBody: &newEmailMsg,
							},
						},
					},
				},
			)
			Expect(err).To(Succeed())

			// for an alert condition, update email to slack notification
			_, err = alertingClient.UpdateEndpointImplementation(ctx,
				&alertingv1alpha.CreateImplementation{
					NotificationId: slack.Id,
					ConditionId: &corev1.Reference{
						Id: idsToCreate["email"],
					},
					Implementation: &alertingv1alpha.EndpointImplementation{
						Implementation: &alertingv1alpha.EndpointImplementation_Slack{
							Slack: &alertingv1alpha.SlackImplementation{
								Title: "Slack title [CI]",
								Text:  "Slack message content [CI]",
							},
						},
					},
				},
			)

			Expect(err).To(Succeed())
		})

		It("Should be able to delete endpoint implementations", func() {
			var slack *alertingv1alpha.AlertEndpointWithId
			var email *alertingv1alpha.AlertEndpointWithId
			existing, err := alertingClient.ListAlertEndpoints(ctx,
				&alertingv1alpha.ListAlertEndpointsRequest{})
			Expect(err).To(Succeed())
			Expect(existing.Items).NotTo(HaveLen(0))
			for _, item := range existing.Items {
				if item.Endpoint.GetSlack() != nil {
					slack = item
				} else if item.Endpoint.GetEmail() != nil {
					email = item
				}
			}
			Expect(slack).NotTo(BeNil())
			Expect(email).NotTo(BeNil())
			Expect(curConfig().Receivers).To(HaveLen(3))
			_, err = alertingClient.DeleteAlertEndpoint(ctx, slack.Id)
			Expect(err).To(Succeed())
			_, err = alertingClient.DeleteAlertEndpoint(ctx, email.Id)
			Expect(err).To(Succeed())

			newItems, err := alertingClient.ListAlertEndpoints(ctx,
				&alertingv1alpha.ListAlertEndpointsRequest{},
			)
			var slackNotSet *alertingv1alpha.AlertEndpointWithId
			var emailNotSet *alertingv1alpha.AlertEndpointWithId
			for _, item := range newItems.Items {
				if item.Id == slack.Id {
					slackNotSet = item
				}
				if item.Id == email.Id {
					emailNotSet = item
				}
			}
			Expect(slackNotSet).To(BeNil())
			Expect(emailNotSet).To(BeNil())
		})
	})

})
