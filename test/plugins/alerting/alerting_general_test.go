package alerting_test

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/templates"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

func ManualReloadEndpointBackend(
	curPort int,
	curCancel context.CancelFunc,
	ctx context.Context,
	path string,
) (newPort int, newCancel context.CancelFunc) {
	if curCancel != nil {
		if curPort == 0 {
			panic("Invalid port")
		}
		// expect it should be available before cancel
		webClient := &alerting.AlertManagerAPI{
			Endpoint: "localhost:" + strconv.Itoa(curPort),
			Route:    "/-/ready",
			Verb:     alerting.GET,
		}
		Eventually(func() error {
			resp, err := http.Get(webClient.ConstructHTTP())
			if err != nil {
				return err
			}
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("status code: %d", resp.StatusCode)
			}
			return nil
		}, time.Second*10, time.Second).Should(Succeed())

		curCancel()
		Eventually(func() error {
			resp, err := http.Get(webClient.ConstructHTTP())
			if err != nil {
				return nil
			}
			if resp.StatusCode == http.StatusNotFound {
				return nil
			}
			return fmt.Errorf("alertmanager api is still running")
		}, time.Second*10, time.Second).Should(Succeed())
	}

	ctxca, ca := context.WithCancel(ctx)
	newPort = env.StartAlertManager(ctxca, path)
	Expect(newPort).NotTo(Equal(0))
	newWebClient := &alerting.AlertManagerAPI{
		Endpoint: "localhost:" + strconv.Itoa(newPort),
		Route:    "/-/ready",
		Verb:     alerting.GET,
	}
	Eventually(func() error {
		resp, err := http.Get(newWebClient.ConstructHTTP())
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status code: %d", resp.StatusCode)
		}
		return nil
	}, time.Second*10, time.Second).Should(Succeed())
	return newPort, ca
}

var _ = Describe("Alerting Backend", Ordered, Label(test.Unit, test.Slow), func() {
	ctx := context.Background()
	// test environment references
	When("The alerting plugin starts with a mocked runtime backend", func() {
		Specify("The mocked runtime backend should be able to start and stop", func() {
			ctxca, ca := context.WithCancel(ctx)
			webPort := env.StartAlertManager(ctxca, alerting.LocalAlertManagerPath)
			webClient := &alerting.AlertManagerAPI{
				Endpoint: "localhost:" + strconv.Itoa(webPort),
				Route:    "/-/ready",
				Verb:     alerting.GET,
			}

			_, err := url.Parse(webClient.ConstructHTTP())
			Expect(err).To(Succeed())
			fmt.Println("Alert manager starting up ...")
			Eventually(func() error {
				resp, err := http.Get(webClient.ConstructHTTP())
				if err != nil {
					return err
				}
				if resp.StatusCode != http.StatusOK {
					return fmt.Errorf("status code: %d", resp.StatusCode)
				}
				return nil
			}, time.Second*10, time.Second).Should(Succeed())

			ca()
			fmt.Println("Alert manager shutting down ...")
			Eventually(func() error {
				resp, err := http.Get(webClient.ConstructHTTP())
				if err != nil {
					return nil
				}
				if resp.StatusCode == http.StatusNotFound {
					return nil
				}
				return fmt.Errorf("alertmanager api is still running")
			}, time.Second*10, time.Second).Should(Succeed())
		})

		Specify("We should be able to hot reload the mocked backend", func() {
			curPort, curCancel := ManualReloadEndpointBackend(0, nil, ctx, alerting.LocalAlertManagerPath)
			newPort, newCancel := ManualReloadEndpointBackend(curPort, curCancel, ctx, alerting.LocalAlertManagerPath)
			Expect(newPort).NotTo(Equal(0))
			newCancel()
			webClient := &alerting.AlertManagerAPI{
				Endpoint: "localhost:" + strconv.Itoa(newPort),
				Route:    "/-/ready",
				Verb:     alerting.GET,
			}
			Eventually(func() error {
				resp, err := http.Get(webClient.ConstructHTTP())
				if err != nil {
					return nil
				}
				if resp.StatusCode == http.StatusNotFound {
					return nil
				}
				return fmt.Errorf("alertmanager api is still running")
			}, time.Second*10, time.Second).Should(Succeed())
		})
	})

	When("The user wants to list available runtime information", func() {
		It("Should list available runtime information they can use in alert descriptions", func() {
			for _, enumValue := range alertingv1alpha.AlertType_value {
				_, err := alertingClient.ListAvailableTemplatesForType(ctx, &alertingv1alpha.AlertDetailChoicesRequest{
					AlertType: alertingv1alpha.AlertType(enumValue),
				})
				Expect(err).NotTo(HaveOccurred())
			}

			resp, err := alertingClient.ListAvailableTemplatesForType(ctx, &alertingv1alpha.AlertDetailChoicesRequest{AlertType: alertingv1alpha.AlertType_SYSTEM})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Template).To(ConsistOf(templates.StrSliceAsTemplates([]string{"agentId", "timeout"})))
		})
	})

	// integration tests for endpoints
	When("When opni alerting dispatches to an endpoint implementation", func() {
		XIt("Should reach its slack target", func() {
			slackWebhook := os.Getenv("OPNI_SLACK_WEBHOOK_INTEGRATION_TEST")
			Expect(slackWebhook).NotTo(Equal(""))
			slackChannel := os.Getenv("OPNI_SLACK_CHANNEL_INTEGRATION_TEST")
			Expect(slackChannel).NotTo(Equal(""))
		})

		XIt("should reach its email target", func() {
			emailTarget := os.Getenv("OPNI_EMAIL_RECEIVER_INTEGRATION_TEST")
			Expect(emailTarget).NotTo(Equal(""))
		})

	})
})
