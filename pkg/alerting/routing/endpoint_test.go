package routing_test

import (
	"bytes"
	"strings"

	"github.com/rancher/opni/pkg/alerting/backend"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cfg "github.com/prometheus/alertmanager/config"
	"github.com/rancher/opni/pkg/alerting/routing"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/test"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	"gopkg.in/yaml.v2"
)

const a = `
route:
  group_by: ['alertname']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 1h
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

func defaultConfig() (*routing.RoutingTree, error) {
	var c routing.RoutingTree
	templateToFill := shared.DefaultAlertManager
	var b bytes.Buffer
	err := templateToFill.Execute(&b, shared.DefaultAlertManagerInfo{
		CortexHandlerName: "web.hook",
		CortexHandlerURL:  "http://127.0.0.1:5001/",
	})
	if err != nil {
		panic(err)
	}
	err = c.Parse(b.String())
	return &c, err
}

var _ = Describe("Internal alerting plugin functionality test", Ordered, Label(test.Unit, test.Slow), func() {
	BeforeAll(func() {
		backend.RuntimeBinaryPath = "../../../../"
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
			Expect(cfg.Route).NotTo(Equal(""))
		})

		It("Should be able to add a variety of receivers to our configmap", func() {
			cfg, err := defaultConfig()
			Expect(err).To(BeNil())

			slackEndpoint := alertingv1alpha.SlackEndpoint{
				Channel:    "#general",
				WebhookUrl: "http://localhost:5001/",
			}
			id1 := uuid.New().String()
			recv, err := routing.NewSlackReceiver(id1, &slackEndpoint)
			Expect(err).To(Succeed())
			cfg.AppendReceiver(recv)
			Expect(cfg.Receivers).To(HaveLen(2))

			fromAddr := "alex7285@gmail.com"
			emailEndpoint := alertingv1alpha.EmailEndpoint{
				To:       "alexandre.lamarre@suse.com",
				SmtpFrom: &fromAddr,
			}
			emailId1 := uuid.New().String()
			emailRecv, err := routing.NewEmailReceiver(emailId1, &emailEndpoint)
			Expect(err).To(Succeed())
			cfg.AppendReceiver(emailRecv)
			Expect(cfg.Receivers).To(HaveLen(3))
			tempId := uuid.New().String()
			emailEndpoint.To = "alexandre.lamarre@suse.com"
			_, err = routing.NewEmailReceiver(tempId, &emailEndpoint)
			Expect(err).To(Succeed())
		})

		It("Should be able to update a receiver", func() {
			cfg, err := defaultConfig()
			Expect(err).To(BeNil())

			slackEndpoint := alertingv1alpha.SlackEndpoint{
				Channel:    "#general",
				WebhookUrl: "http://localhost:5001/",
			}
			id1 := uuid.New().String()
			recv, err := routing.NewSlackReceiver(id1, &slackEndpoint)
			Expect(err).To(Succeed())
			cfg.AppendReceiver(recv)
			Expect(cfg.Receivers).To(HaveLen(2))
			Expect(cfg.Receivers[1].Name).To(Equal(id1))
			target := &alertingv1alpha.SlackEndpoint{
				Channel:    "#somethingelse",
				WebhookUrl: "http://localhost:5001/",
			}
			newRecv, err := routing.NewSlackReceiver(id1, target)
			Expect(err).To(Succeed())
			err = cfg.UpdateReceiver(id1, newRecv)
			Expect(err).To(Succeed())
			Expect(cfg.Receivers).To(HaveLen(2))
			Expect(cfg.Receivers[1].Name).To(Equal(id1))
			Expect(cfg.Receivers[1].SlackConfigs).NotTo(BeNil())
			Expect(cfg.Receivers[1].SlackConfigs[0].Channel).To(Equal("#somethingelse"))
		})

		It("Should be able to update one receiver type to another", func() {
			cfg, err := defaultConfig()
			Expect(err).To(BeNil())

			slackEndpoint := alertingv1alpha.SlackEndpoint{
				Channel:    "#general",
				WebhookUrl: "http://localhost:5001/",
			}
			id1 := uuid.New().String()
			recv, err := routing.NewSlackReceiver(id1, &slackEndpoint)
			Expect(err).To(Succeed())
			cfg.AppendReceiver(recv)
			Expect(cfg.Receivers).To(HaveLen(2))
			Expect(cfg.Receivers[1].Name).To(Equal(id1))

			// udpate

			recv, err = routing.NewEmailReceiver(id1, &alertingv1alpha.EmailEndpoint{
				To: "alexandre.lamarre@suse.com",
			})
			Expect(err).To(Succeed())
			err = cfg.UpdateReceiver(id1, recv)
			Expect(err).To(Succeed())
			Expect(cfg.Receivers).To(HaveLen(2))
			Expect(cfg.Receivers[1].Name).To(Equal(id1))
			Expect(cfg.Receivers[1].EmailConfigs).NotTo(BeNil())
			Expect(cfg.Receivers[1].SlackConfigs).To(BeNil())
		})

		It("Should fail when updating receivers out of bounds", func() {
			cfg, err := defaultConfig()
			Expect(err).To(BeNil())
			newRecv, err := routing.NewSlackReceiver(
				uuid.New().String(),
				&alertingv1alpha.SlackEndpoint{
					Channel:    "#general",
					WebhookUrl: "http://localhost:5001/",
				},
			)
			Expect(err).To(Succeed())
			err = cfg.UpdateReceiver(uuid.New().String(), newRecv)
			Expect(err).To(HaveOccurred())
		})

		It("Should fail when deleting receivers out of bounds", func() {
			cfg, err := defaultConfig()
			Expect(err).To(BeNil())
			err = cfg.DeleteReceiver(uuid.New().String())
			Expect(err).To(HaveOccurred())
		})

		Specify("Deleting receivers should succeed when they are the only element", func() {
			cfg, err := defaultConfig()
			Expect(err).To(BeNil())
			err = cfg.DeleteReceiver("web.hook")
			Expect(err).NotTo(HaveOccurred())
		})

		Specify("Deleting receivers should succeed when they are the first element", func() {
			cfg, err := defaultConfig()
			Expect(err).To(BeNil())
			for i := 1; i <= 20; i++ {
				slackEndpoint := alertingv1alpha.SlackEndpoint{
					Channel:    "#general",
					WebhookUrl: "http://localhost:5001/",
				}
				id1 := uuid.New().String()
				recv, err := routing.NewSlackReceiver(id1, &slackEndpoint)
				Expect(err).To(Succeed())
				cfg.AppendReceiver(recv)
			}
			Expect(cfg.Receivers).To(HaveLen(21))
			err = cfg.DeleteReceiver("web.hook")
			Expect(err).To(Succeed())

			Expect(cfg.Receivers[0].Name).NotTo(Equal("web.hook"))
			Expect(cfg.Receivers).To(HaveLen(20))
		})

		Specify("Deleting receivers should succeed when they are the last element", func() {
			cfg, err := defaultConfig()
			Expect(err).To(BeNil())
			var lastId string
			for i := 1; i <= 20; i++ {
				slackEndpoint := alertingv1alpha.SlackEndpoint{
					Channel:    "#general",
					WebhookUrl: "http://localhost:5001/",
				}
				id1 := uuid.New().String()
				recv, err := routing.NewSlackReceiver(id1, &slackEndpoint)
				Expect(err).To(Succeed())
				cfg.AppendReceiver(recv)
				lastId = id1
			}
			Expect(cfg.Receivers).To(HaveLen(21))
			err = cfg.DeleteReceiver(lastId)
			Expect(err).To(Succeed())
			Expect(cfg.Receivers).To(HaveLen(20))
			Expect(cfg.Receivers[19].Name).NotTo(Equal(lastId))
		})
	})
})
