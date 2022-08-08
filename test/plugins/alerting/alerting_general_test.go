package alerting_test

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
	"net/http"
	"net/url"
	"strconv"
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
			webPort := env.StartAlertManager(ctxca, alerting.LocalAlertManagerPath)
			webClient := &alerting.AlertManagerAPI{
				Endpoint: "localhost:" + strconv.Itoa(webPort),
				Route:    "/-/ready",
				Verb:     alerting.GET,
			}

			_, err := url.Parse(webClient.ConstructHTTP())
			Expect(err).To(Succeed())

			resp, err := http.Get(webClient.ConstructHTTP())
			Expect(err).To(Succeed())
			Expect(resp.StatusCode).To(Equal(200))
			defer ca()
		})

	})
})
