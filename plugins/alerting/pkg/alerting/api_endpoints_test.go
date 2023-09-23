package alerting_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/samber/lo"

	"github.com/rancher/opni/pkg/test"
)

var _ = Describe("Converting ServiceLevelObjective Messages to Prometheus Rules", Ordered, Label("integration", "slow"), func() {
	ctx := context.Background()
	var env *test.Environment
	var endpointClient alertingv1.AlertEndpointsClient

	BeforeAll(func() {
		env = &test.Environment{}
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop, "Test Suite Finished")
		endpointClient = alertingv1.NewAlertEndpointsClient(env.ManagementClientConn())
		Expect(endpointClient).NotTo(BeNil())

	})

	When("CRUDing Alert endpoints", func() {
		It("should redact/unredact secrets appropriately", func() {
			originalPg := &alertingv1.AlertEndpoint{
				Name:        "some-name",
				Description: "some-description",
				Endpoint: &alertingv1.AlertEndpoint_PagerDuty{
					PagerDuty: &alertingv1.PagerDutyEndpoint{
						IntegrationKey: "some-key",
					},
				},
			}
			uuid, err := endpointClient.CreateAlertEndpoint(ctx, originalPg)
			Expect(err).NotTo(HaveOccurred())
			Expect(uuid).NotTo(BeNil())
			Expect(uuid.Id).NotTo(Equal(""))

			endp, err := endpointClient.GetAlertEndpoint(ctx, uuid)
			Expect(err).NotTo(HaveOccurred())
			Expect(endp).NotTo(BeNil())
			Expect(endp.GetPagerDuty()).NotTo(BeNil())
			Expect(endp.GetPagerDuty().GetIntegrationKey()).To(Equal(storagev1.Redacted))
			endp.UnredactSecrets(originalPg)
			Expect(endp.GetPagerDuty().GetIntegrationKey()).To(Equal("some-key"))

			originalSlack := &alertingv1.AlertEndpoint{
				Name:        "some-name",
				Description: "some-description",
				Endpoint: &alertingv1.AlertEndpoint_Slack{
					Slack: &alertingv1.SlackEndpoint{
						WebhookUrl: "http://mock-webhook-url",
						Channel:    "#some-channel",
					},
				},
			}
			uuid2, err := endpointClient.CreateAlertEndpoint(ctx, originalSlack)
			Expect(err).NotTo(HaveOccurred())
			Expect(uuid).NotTo(BeNil())
			Expect(uuid.Id).NotTo(Equal(""))
			endp2, err := endpointClient.GetAlertEndpoint(ctx, uuid2)
			Expect(err).NotTo(HaveOccurred())
			Expect(endp2).NotTo(BeNil())
			Expect(endp2.GetSlack()).NotTo(BeNil())
			Expect(endp2.GetSlack().GetWebhookUrl()).To(Equal(storagev1.Redacted))
			endp2.UnredactSecrets(originalSlack)
			Expect(endp2.GetSlack().GetWebhookUrl()).To(Equal("http://mock-webhook-url"))

			originalEmail := &alertingv1.AlertEndpoint{
				Name:        "some-name",
				Description: "some-description",
				Endpoint: &alertingv1.AlertEndpoint_Email{
					Email: &alertingv1.EmailEndpoint{
						To:               test.RandomName(time.Now().UnixNano()) + "@gmail.com",
						SmtpFrom:         lo.ToPtr("bot@opni.com"),
						SmtpSmartHost:    lo.ToPtr("server:6000"),
						SmtpAuthUsername: lo.ToPtr("alex"),
						SmtpAuthPassword: lo.ToPtr("password"),
					},
				},
			}
			uuid3, err := endpointClient.CreateAlertEndpoint(ctx, originalEmail)
			Expect(err).NotTo(HaveOccurred())
			Expect(uuid).NotTo(BeNil())
			Expect(uuid.Id).NotTo(Equal(""))
			endp3, err := endpointClient.GetAlertEndpoint(ctx, uuid3)
			Expect(err).NotTo(HaveOccurred())
			Expect(endp3).NotTo(BeNil())
			Expect(endp3.GetEmail()).NotTo(BeNil())
			Expect(endp3.GetEmail().GetSmtpAuthPassword()).To(Equal(storagev1.Redacted))
			endp3.UnredactSecrets(originalEmail)
			Expect(endp3.GetEmail().GetSmtpAuthPassword()).To(Equal("password"))
		})
	})
})
