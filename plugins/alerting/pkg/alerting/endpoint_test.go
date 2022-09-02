package alerting_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/phayes/freeport"

	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/logger"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/alertmanager/config"
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
			recv, err := alerting.NewSlackReceiver(id1, &slackEndpoint)
			Expect(err).To(Succeed())
			cfg.AppendReceiver(recv)
			Expect(cfg.Receivers).To(HaveLen(2))
			slackEndpoint.Channel = "something that doesn't have a prefix"
			_, err = alerting.NewSlackReceiver(id1, &slackEndpoint)
			Expect(err).To(HaveOccurred())

			fromAddr := "alex7285@gmail.com"
			emailEndpoint := alertingv1alpha.EmailEndpoint{
				To:   "alexandre.lamarre@suse.com",
				From: &fromAddr,
			}
			emailId1 := uuid.New().String()
			emailRecv, err := alerting.NewEmailReceiver(emailId1, &emailEndpoint)
			Expect(err).To(Succeed())
			cfg.AppendReceiver(emailRecv)
			Expect(cfg.Receivers).To(HaveLen(3))
			emailEndpoint.To = "alexandre.com"
			_, err = alerting.NewEmailReceiver(emailId1, &emailEndpoint)
			Expect(err).To(HaveOccurred())
			tempId := uuid.New().String()
			emailEndpoint.To = "alexandre.lamarre@suse.com"
			_, err = alerting.NewEmailReceiver(tempId, &emailEndpoint)
			Expect(err).To(Succeed())
			fromAddr = "invalid.email.com"
			emailEndpoint.From = &fromAddr
			_, err = alerting.NewEmailReceiver(tempId, &emailEndpoint)
			Expect(err).To(HaveOccurred())
		})

		It("Should be able to update a receiver", func() {
			cfg, err := defaultConfig()
			Expect(err).To(BeNil())

			slackEndpoint := alertingv1alpha.SlackEndpoint{
				Channel:    "#general",
				WebhookUrl: "http://localhost:5001/",
			}
			id1 := uuid.New().String()
			recv, err := alerting.NewSlackReceiver(id1, &slackEndpoint)
			Expect(err).To(Succeed())
			cfg.AppendReceiver(recv)
			Expect(cfg.Receivers).To(HaveLen(2))
			Expect(cfg.Receivers[1].Name).To(Equal(id1))
			target := &alertingv1alpha.SlackEndpoint{
				Channel:    "#somethingelse",
				WebhookUrl: "http://localhost:5001/",
			}
			newRecv, err := alerting.NewSlackReceiver(id1, target)
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
			recv, err := alerting.NewSlackReceiver(id1, &slackEndpoint)
			Expect(err).To(Succeed())
			cfg.AppendReceiver(recv)
			Expect(cfg.Receivers).To(HaveLen(2))
			Expect(cfg.Receivers[1].Name).To(Equal(id1))

			// udpate

			recv, err = alerting.NewEmailReceiver(id1, &alertingv1alpha.EmailEndpoint{
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
			newRecv, err := alerting.NewSlackReceiver(
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
				recv, err := alerting.NewSlackReceiver(id1, &slackEndpoint)
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
				recv, err := alerting.NewSlackReceiver(id1, &slackEndpoint)
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

	When("We modify AlertManager global settings", func() {
		It("should be able to set/unset SMTP host settings", func() {
			cfg, err := defaultConfig()
			Expect(err).To(BeNil())
			cfg.SetDefaultSMTPServer()
			Expect(cfg.Global.SMTPHello).To(Equal(alerting.DefaultSMTPServerHost))
			Expect(cfg.Global.SMTPSmarthost).To(Equal(config.HostPort{Port: fmt.Sprintf("%d", alerting.DefaultSMTPServerPort)}))

			cfg.UnsetSMTPServer()
			Expect(cfg.Global.SMTPHello).To(Equal(""))
			Expect(cfg.Global.SMTPSmarthost).To(Equal(config.HostPort{}))
		})

		It("Should be able to reconcile errors concerning SMTP not set", func() {
			cfg, err := defaultConfig()
			Expect(err).To(BeNil())
			fromAddr := "bot@google.com"
			emailEndpoint := alertingv1alpha.EmailEndpoint{
				To:   "alexandre.lamarre@suse.com",
				From: &fromAddr,
			}
			emailId1 := uuid.New().String()
			emailRecv, err := alerting.NewEmailReceiver(emailId1, &emailEndpoint)
			Expect(err).To(Succeed())
			cfg.AppendReceiver(emailRecv)
			raw, err := cfg.Marshal()
			Expect(err).To(BeNil())
			reconcileErr := alerting.ValidateIncomingConfig(string(raw), logger.NewPluginLogger().Named("alerting"))
			Expect(reconcileErr).To(HaveOccurred())
			expectedError := alerting.NoSmartHostSet
			Expect(reconcileErr.Error()).To(Equal(expectedError))
			err = alerting.ReconcileInvalidState(cfg, reconcileErr)
			Expect(err).To(Succeed())
			Expect(cfg.Global.SMTPHello).To(Equal(alerting.DefaultSMTPServerHost))
			Expect(cfg.Global.SMTPSmarthost).To(Equal(
				config.HostPort{Port: fmt.Sprintf("%d",
					alerting.DefaultSMTPServerPort)}))
		})

		It("Should apply a reconciler loop successfully to an STMP host not set", func() {
			//TODO
		})

		It("Should be able to load our default configuration file", func() {
			mux := http.NewServeMux()

			port, err := freeport.GetFreePort()
			Expect(err).To(Succeed())
			mux.HandleFunc(shared.AlertingCortexHookHandler, func(w http.ResponseWriter, r *http.Request) {})
			address := fmt.Sprintf("127.0.0.1:%d", port)
			exampleHookServer := &http.Server{
				Addr:           address,
				Handler:        mux,
				ReadTimeout:    1 * time.Second,
				WriteTimeout:   1 * time.Second,
				MaxHeaderBytes: 1 << 20,
			}
			go func() {
				err := exampleHookServer.ListenAndServe()
				if !errors.Is(err, http.ErrServerClosed) {
					panic(err)
				}
			}()
			defer exampleHookServer.Shutdown(context.Background())
			urlStr := "http://" + address
			u, err := url.Parse(urlStr)
			Expect(u.Host).NotTo(Equal(""))
			Expect(err).To(BeNil())
			err = shared.BackendDefaultFile(urlStr)
			Expect(err).To(BeNil())
			bytes, err := os.ReadFile("/tmp/alertmanager.yaml")
			Expect(err).To(Succeed())
			c := &alerting.ConfigMapData{}
			err = c.Parse(string(bytes))
			Expect(err).To(Succeed())
		})
	})
})
