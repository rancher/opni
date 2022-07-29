package alerting_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cfg "github.com/prometheus/alertmanager/config"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
	"gopkg.in/yaml.v2"
)

const a = `
route:
  group_by: ['alertname']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 1h
  receiver: 'web.hook'
`

const b = `
receivers:
  - name: 'web.hook'
    webhook_configs:
      - url: 'http://127.0.0.1:5001/'
`

const c = `
inhibit_rules:
- source_match:
  severity: 'critical'
  target_match:
  severity: 'warning'
  equal: ['alertname', 'dev', 'instance']`

func defaultConfig() (*alerting.ConfigMapData, error) {
	var c alerting.ConfigMapData
	err := c.Parse(alerting.DefaultAlertManager)
	return &c, err
}

var _ = Describe("Internal alerting plugin functionality test", Ordered, Label(test.Unit, test.Slow), func() {
	BeforeAll(func() {

	})

	When("We modify config map data in the api", func() {

		It("should be able to unmarshal prometheus structs", func() {

			var route cfg.Route
			err := yaml.Unmarshal([]byte(strings.TrimSpace(a)), &route)
			Expect(err).To(BeNil())

			// var receivers []*cfg.Receiver
			// err = yaml.Unmarshal(
			// 	[]byte(b),
			// 	&receivers)
			// Expect(err).To(BeNil())

			var inhibit cfg.InhibitRule
			err = yaml.Unmarshal([]byte(strings.TrimSpace(c)), &inhibit)
			Expect(err).To(BeNil())
		})

		It("Should be able to unmarshal our AlertManager configmap", func() {
			cfg, err := defaultConfig()
			Expect(err).To(BeNil())
			Expect(cfg.Receivers).To(HaveLen(1))
			Expect(cfg.Receivers[0].Name).To(Equal("web.hook"))
			Expect(cfg.Receivers[0].WebhookConfigs).To(HaveLen(1))
			Expect(cfg.InhibitRules).To(HaveLen(1))
			Expect(cfg.Route).ToNot(Equal(""))
		})

		It("Should be able to add a variety of receivers to our configmap", func() {
			cfg, err := defaultConfig()
			Expect(err).To(BeNil())

			slackEndpoint := alertingv1alpha.SlackEndpoint{
				Name:    "slack",
				Channel: "#general",
				ApiUrl:  "http://localhost:5001/",
			}
			recv, err := alerting.NewSlackReceiver(&slackEndpoint)
			Expect(err).To(BeNil())
			cfg.AppendReceiver(recv)
			Expect(cfg.Receivers).To(HaveLen(2))
			slackEndpoint.Channel = "something that doesn't have a prefix"
			_, err = alerting.NewSlackReceiver(&slackEndpoint)
			Expect(err).ToNot(BeNil())

			fromAddr := "alex7285@gmail.com"
			emailEndpoint := alertingv1alpha.EmailEndpoint{
				Name: "email",
				To:   "alexandre.lamarre@suse.com",
				From: &fromAddr,
			}
			emailRecv, err := alerting.NewMailReceiver(&emailEndpoint)
			Expect(err).To(BeNil())
			cfg.AppendReceiver(emailRecv)
			Expect(cfg.Receivers).To(HaveLen(3))
			emailEndpoint.To = "alexandre.com"
			_, err = alerting.NewMailReceiver(&emailEndpoint)
			Expect(err).ToNot(BeNil())
			emailEndpoint.To = "alexandre.lamarre@suse.com"
			_, err = alerting.NewMailReceiver(&emailEndpoint)
			Expect(err).To(BeNil())
			fromAddr = "invalid.email.com"
			emailEndpoint.From = &fromAddr
			_, err = alerting.NewMailReceiver(&emailEndpoint)
			Expect(err).ToNot(BeNil())

		})
	})

	When("We use the alertmanager API", func() {
		It("Should construct the correct URL", func() {
			url := (&alerting.AlertManagerAPI{
				Endpoint: "localhost:9093",
				Route:    "/alerts",
				Verb:     alerting.POST,
			}).WithHttpV2()
			Expect(url.Construct()).To(Equal("localhost:9093/api/v2/alerts"))
			url.WithHttpV1()
			Expect(url.Construct()).To(Equal("localhost:9093/api/v1/alerts"))
		})
	})
})
