package v1_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	storagev1 "github.com/rancher/opni/pkg/apis/storage/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
)

var _ = Describe("Redacting opni alerting secrets", Ordered, Label("unit", "slow"), func() {
	When("we use protobuf messages for opni alerting", func() {
		It("should redact/undredact secrets when appropriate", func() {
			originalPg := &alertingv1.AlertEndpoint{
				Name:        "some-name",
				Description: "some-description",
				Endpoint: &alertingv1.AlertEndpoint_PagerDuty{
					PagerDuty: &alertingv1.PagerDutyEndpoint{
						IntegrationKey: "some-key",
					},
				},
			}
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
			originalPgCopy := util.ProtoClone(originalPg)
			originalSlackCopy := util.ProtoClone(originalSlack)
			originalEmailCopy := util.ProtoClone(originalEmail)

			originalPg.RedactSecrets()
			originalSlack.RedactSecrets()
			originalEmail.RedactSecrets()

			Expect(originalPg.GetPagerDuty().IntegrationKey).To(Equal(storagev1.Redacted))
			Expect(originalSlack.GetSlack().WebhookUrl).To(Equal(storagev1.Redacted))
			Expect(*originalEmail.GetEmail().SmtpAuthPassword).To(Equal(storagev1.Redacted))

			originalPg.UnredactSecrets(originalPgCopy)
			originalSlack.UnredactSecrets(originalSlackCopy)
			originalEmail.UnredactSecrets(originalEmailCopy)

			Expect(originalPg.GetPagerDuty().IntegrationKey).To(Equal(originalPgCopy.GetPagerDuty().IntegrationKey))
			Expect(originalSlack.GetSlack().WebhookUrl).To(Equal(originalSlackCopy.GetSlack().WebhookUrl))
			Expect(*originalEmail.GetEmail().SmtpAuthPassword).To(Equal(*originalEmailCopy.GetEmail().SmtpAuthPassword))
		})
	})
})
