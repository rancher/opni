package backend_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/phayes/freeport"
	cfg "github.com/prometheus/alertmanager/config"
	"github.com/rancher/opni/pkg/alerting/backend"
	"github.com/rancher/opni/pkg/alerting/routing"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/test"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"

	//"github.com/rancher/opni/pkg/test"
	"net/http"
	"net/url"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

	})

	When("We use the alertmanager API", func() {
		It("Should construct the correct URL", func() {
			url := (&backend.AlertManagerAPI{
				Endpoint: "localhost:9093",
				Route:    "/alerts",
				Verb:     backend.POST,
			}).WithHttpV2()
			Expect(url.Construct()).To(Equal("localhost:9093/api/v2/alerts"))
			url.WithHttpV1()
			Expect(url.Construct()).To(Equal("localhost:9093/api/v1/alerts"))
		})
	})

	When("We modify AlertManager global settings", func() {
		It("should be able to set/unset SMTP host settings", func() {
			testcfg, err := defaultConfig()
			Expect(err).To(BeNil())
			testcfg.SetDefaultSMTPServer()
			Expect(testcfg.Global.SMTPHello).To(Equal(routing.DefaultSMTPServerHost))
			Expect(testcfg.Global.SMTPSmarthost).To(Equal(
				cfg.HostPort{
					Port: fmt.Sprintf("%d", routing.DefaultSMTPServerPort),
				}))

			testcfg.UnsetSMTPServer()
			Expect(testcfg.Global.SMTPHello).To(Equal(""))
			Expect(testcfg.Global.SMTPSmarthost).To(Equal(cfg.HostPort{}))
		})

		It("Should be able to reconcile errors concerning SMTP not set", func() {
			testcfg, err := defaultConfig()
			Expect(err).To(BeNil())
			fromAddr := "bot@google.com"
			emailEndpoint := alertingv1alpha.EmailEndpoint{
				To:       "alexandre.lamarre@suse.com",
				SmtpFrom: &fromAddr,
			}
			emailId1 := uuid.New().String()
			emailRecv, err := routing.NewEmailReceiver(emailId1, &emailEndpoint)
			Expect(err).To(Succeed())
			testcfg.AppendReceiver(emailRecv)
			raw, err := testcfg.Marshal()
			Expect(err).To(BeNil())
			reconcileErr := backend.ValidateIncomingConfig(string(raw), logger.NewPluginLogger().Named("alerting"))
			Expect(reconcileErr).To(HaveOccurred())
			expectedError := backend.NoSmartHostSet
			Expect(reconcileErr.Error()).To(Equal(expectedError))
			err = backend.ReconcileInvalidState(testcfg, reconcileErr)
			Expect(err).To(Succeed())
			Expect(testcfg.Global.SMTPHello).To(Equal(routing.DefaultSMTPServerHost))
			Expect(testcfg.Global.SMTPSmarthost).To(Equal(
				cfg.HostPort{Port: fmt.Sprintf("%d",
					routing.DefaultSMTPServerPort)}))
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
			c := &routing.RoutingTree{}
			err = c.Parse(string(bytes))
			Expect(err).To(Succeed())
		})
	})

})
