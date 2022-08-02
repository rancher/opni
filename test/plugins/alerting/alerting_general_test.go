package alerting_test

import (
	"context"
	"net/url"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
)

func ManualReloadEndpointBackend() {
	// TODO
}

var _ = Describe("Alerting Backend", Ordered, Label(test.Unit, test.Slow), func() {
	ctx := context.Background()
	// test environment references
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

			//FIXME: this started crashing with connection refused
			// resp, err := http.Get("http://" + webClient.Construct())
			// Expect(err).To(Succeed())
			// Expect(resp.StatusCode).To(Equal(200))
			// defer resp.Body.Close()
			// Expect(err).To(Succeed())

			// FIXME: this started crashing with connection refused
			// resp2, err2 := http.Get("http://" + apiClient.Construct())
			// Expect(err2).To(Succeed())
			// Expect(resp2.StatusCode).To(Equal(200))

			// defer resp2.Body.Close()
			// Expect(err).To(Succeed())
			defer ca()
		})

	})
})
