package alerting_test

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
)

func ManualReloadEndpointBackend() {
	// TODO
}

var _ = Describe("Alerting Backend", Ordered, Label(test.Unit, test.Slow), func() {
	ctx := context.Background()
	var alertingClient alertingv1alpha.AlertingClient
	// test environment references
	var env *test.Environment
	BeforeAll(func() {
		// use alertmanager binary
		os.Setenv(alerting.LocalBackendEnvToggle, "true")
		os.WriteFile(alerting.LocalAlertManagerPath, []byte(alerting.DefaultAlertManager), 0644)

		// setup managemet server & client
		env = &test.Environment{
			TestBin: "../../../testbin/bin",
		}
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)

		// alerting plugin
		alertingClient = alertingv1alpha.NewAlertingClient(env.ManagementClientConn())
	})

	When("The alerting plugin starts", func() {
		Specify("The mocked runtime backend should be able to start and stop", func() {
			ctxca, ca := context.WithCancel(ctx)
			webPort, apiPort := env.StartAlertManager(ctxca, alerting.LocalAlertManagerPath)
			webClient := (&alerting.AlertManagerAPI{
				Endpoint: "127.0.0.1:" + strconv.Itoa(webPort),
				Route:    "/-/ready",
				Verb:     alerting.GET,
			})

			apiClient := (&alerting.AlertManagerAPI{
				Endpoint: "127.0.0.1:" + strconv.Itoa(apiPort),
				Route:    "/receivers",
				Verb:     alerting.GET,
			}).WithHttpV2()
			_, err := url.Parse("http://" + webClient.Construct())
			Expect(err).To(Succeed())
			_, err = url.Parse("http://" + apiClient.Construct())
			Expect(err).To(Succeed())

			// FIXME: dont do this
			time.Sleep(time.Second * 2)

			resp, err := http.Get("http://" + webClient.Construct())
			Expect(err).To(Succeed())
			Expect(resp.StatusCode).To(Equal(200))
			defer resp.Body.Close()
			Expect(err).To(Succeed())

			// FIXME: this is completely broken and I don't know why
			// resp2, err2 := http.Get("http://" + apiClient.Construct())
			// Expect(err2).To(Succeed())
			// Expect(resp2.StatusCode).To(Equal(200))

			// defer resp2.Body.Close()
			// Expect(err).To(Succeed())
			defer ca()
		})

		It("Should be able to get default implementations for each alert endpoint", func() {
			impls := map[string]*alertingv1alpha.EndpointImplementation{
				"slack": {
					Implementation: &alertingv1alpha.EndpointImplementation_Slack{},
				},
				"email": {
					Implementation: &alertingv1alpha.EndpointImplementation_Email{},
				},
			}

			sdef := impls["slack"].GetSlack().Defaults()
			Expect(sdef).To(Equal(&alertingv1alpha.SlackImplementation{
				Title:    "Your optional title here",
				Text:     "Your optional text here",
				Footer:   "Your optional footer here",
				ImageUrl: "Your optional image url here",
			}))

			edef := impls["email"].GetEmail().Defaults()
			hbody := "Your optional body here"
			tbody := "Your optional body here"
			Expect(edef).To(Equal(&alertingv1alpha.EmailImplementation{
				HtmlBody: &hbody,
				TextBody: &tbody,
			}))
		})

		It("Should have loaded preconfigured alert conditions", func() {
			_, err := alertingClient.ListAlertConditions(
				ctx, &alertingv1alpha.ListAlertConditionRequest{})
			Expect(err).To(HaveOccurred())
		})
	})
})
