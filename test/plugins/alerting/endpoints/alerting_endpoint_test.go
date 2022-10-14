package endpoints_test

import (
	"context"
	"fmt"
	"os"

	"github.com/rancher/opni/pkg/alerting/config"

	"github.com/rancher/opni/pkg/alerting/shared"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"

	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
)

func curConfig() *config.ConfigMapData {
	curConfigData, err := os.ReadFile(shared.LocalAlertManagerPath)
	Expect(err).To(Succeed())
	curConfig := string(curConfigData)
	configMap := &config.ConfigMapData{}
	err = configMap.Parse(curConfig)
	Expect(err).To(Succeed())
	return configMap
}

var idsToCreate = map[string]string{"slack": uuid.New().String(), "email": uuid.New().String()}

var _ = Describe("Alerting Endpoints integration tests", Ordered, Label(test.Unit, test.Slow), func() {
	ctx := context.Background()
	BeforeEach(func() {
		alerting.AlertPath = "../../../dev/alerttestdata/logs"
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
								WebhookUrl: "not a url",
								Channel:    "#general",
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
								WebhookUrl: "http://some.url.com",
								Channel:    "not a channel",
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
								SmtpFrom: &fromUrl,
								To:       "",
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
								SmtpFrom: &fromUrl,
								To:       "asdasdaasdasd",
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
								SmtpFrom: &notFromUrl,
								To:       "alexandre.lamarre@suse.com",
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
		Specify("Cleaning up edge case data", func() {
			defaultCfg, err := defaultConfig()
			Expect(err).To(Succeed())
			err = os.WriteFile(shared.LocalAlertManagerPath, defaultCfg.Bytes(), 0644)
			Expect(err).To(Succeed())
		})

	})

	When("The alerting plugin starts", func() {
		It("Should be able to CRUD (Reusable K,V groups) Alert Endpoints", func() {
			fromUrl := "bot@google.com"

			// Note : do not use `#general` slack channel when testing, since if something goes wrong
			// with AM internally, it will default back to #general
			inputs := []*alertingv1alpha.AlertEndpoint{
				{
					Name:        "TestAlertEndpoint",
					Description: "TestAlertEndpoint",
					Endpoint: &alertingv1alpha.AlertEndpoint_Email{
						Email: &alertingv1alpha.EmailEndpoint{
							SmtpFrom: nil,
							To:       "alex7285@gmail.com",
						},
					},
				},
				{
					Name:        "TestAlertEndpoint2",
					Description: "TestAlertEndpoint2",
					Endpoint: &alertingv1alpha.AlertEndpoint_Email{
						Email: &alertingv1alpha.EmailEndpoint{
							To:       "alexandre.lamarre@suse.com",
							SmtpFrom: &fromUrl,
						},
					},
				},
				{
					Name:        "TestAlertEndpoint3",
					Description: "TestAlertEndpoint3",
					Endpoint: &alertingv1alpha.AlertEndpoint_Slack{
						Slack: &alertingv1alpha.SlackEndpoint{
							Channel:    "#channel",
							WebhookUrl: "https://hooks.slack.com/services/T0S0S0S0S/B0S0S0S0S/B0S0S0S0S",
						},
					},
				},
				{
					Name:        "TestAlertEndpoint4",
					Description: "TestAlertEndpoint4",
					Endpoint: &alertingv1alpha.AlertEndpoint_Slack{
						Slack: &alertingv1alpha.SlackEndpoint{
							Channel:    "#another-channel",
							WebhookUrl: "https://hooks.slack.com/services/AAAAAAAA/B0S0S0S0S/B0S0S0S0S",
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
			actual, err := alertingClient.ListAlertEndpoints(ctx, &alertingv1alpha.ListAlertEndpointsRequest{})
			Expect(err).To(Succeed())
			for _, input := range inputs {
				found := false
				for _, output := range actual.Items {
					if input.Name == output.Endpoint.Name {
						if input.GetEmail() != nil {
							Expect(input.GetEmail().To).To(Equal(output.Endpoint.GetEmail().To))
							Expect(input.GetEmail().SmtpFrom).To(Equal(output.Endpoint.GetEmail().SmtpFrom))
						}
						if input.GetSlack() != nil {
							Expect(input.GetSlack().Channel).To(Equal(output.Endpoint.GetSlack().Channel))
							Expect(input.GetSlack().WebhookUrl).To(Equal(output.Endpoint.GetSlack().WebhookUrl))
						}
						found = true
						break
					}
				}
				Expect(found).To(BeTrue())
			}
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
							To: emailTo,
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
							Channel:    slackChannel,
							WebhookUrl: slackApiUrl,
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
			Expect(found.Endpoint.GetSlack().Channel).To(Equal(slackChannel))
			Expect(found.Endpoint.GetSlack().WebhookUrl).To(Equal(slackApiUrl))

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
					EndpointId: slack.Id,
					ConditionId: &corev1.Reference{
						Id: idsToCreate["slack"],
					},
					Implementation: &alertingv1alpha.EndpointImplementation{
						Title: "slack endpoint",
						Body:  "hello world",
					},
				},
			)

			Expect(err).To(Succeed())
			Expect(curConfig().Receivers).To(HaveLen(2))
			foundSlack := false
			for _, recv := range curConfig().Receivers {
				if recv.Name == idsToCreate["slack"] {
					Expect(recv.SlackConfigs).To(HaveLen(1))
					Expect(recv.SlackConfigs[0].Channel).To(Equal(slack.Endpoint.GetSlack().Channel))
					Expect(recv.SlackConfigs[0].APIURL).To(Equal(slack.Endpoint.GetSlack().GetWebhookUrl()))
					foundSlack = true
				}
			}
			Expect(foundSlack).To(BeTrue())
			// check configuration

			emailContent := "Email message content [CI]"
			_, err = alertingClient.CreateEndpointImplementation(ctx,
				&alertingv1alpha.CreateImplementation{
					EndpointId: email.Id,
					ConditionId: &corev1.Reference{
						Id: idsToCreate["email"],
					},
					Implementation: &alertingv1alpha.EndpointImplementation{
						Title: "asasas",
						Body:  emailContent,
					},
				},
			)
			Expect(err).To(Succeed())
			configMap := curConfig().Receivers
			Expect(configMap).To(HaveLen(3))
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
					EndpointId: email.Id,
					ConditionId: &corev1.Reference{
						Id: idsToCreate["slack"],
					},
					Implementation: &alertingv1alpha.EndpointImplementation{
						Title: "email endpoint",
						Body:  newEmailMsg,
					},
				},
			)
			Expect(err).To(Succeed())

			// for an alert condition, update email to slack notification
			_, err = alertingClient.UpdateEndpointImplementation(ctx,
				&alertingv1alpha.CreateImplementation{
					EndpointId: slack.Id,
					ConditionId: &corev1.Reference{
						Id: idsToCreate["email"],
					},
					Implementation: &alertingv1alpha.EndpointImplementation{

						Title: " new title",
						Body:  " new body",
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
