package backend_test

import (
	"context"
	"encoding/json"
	"net/url"
	"time"

	backoffv2 "github.com/lestrrat-go/backoff/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/backend"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
)

type apiConfigRequest struct {
	RawConfig string `json:"config"`
}

var _ = Describe("Alert manager adapter tests", Ordered, Label(test.Unit, test.Slow), func() {
	When("We create a client for connect to the internal opni alert manager server", func() {
		It("should set to valid default values", func() {

			pipelineRetrier := backoffv2.Exponential(
				backoffv2.WithMinInterval(time.Second*2),
				backoffv2.WithMaxInterval(time.Second*5),
				backoffv2.WithMaxRetries(3),
				backoffv2.WithMultiplier(1.2),
			)

			ctx := context.Background()
			config := util.Must(shared.DefaultAlertManagerConfig("localhost:3000"))
			strConfig := config.String()
			a := backend.NewAlertManagerOpniConfigClient(ctx,
				"localhost:3000",
				backend.WithRetrier(pipelineRetrier),
				backend.WithRequestBody(

					util.Must(
						json.Marshal(apiConfigRequest{RawConfig: strConfig}),
					),
				),
			)
			httpEndpoint := a.ConstructHTTP()
			Expect(httpEndpoint).NotTo(BeNil())
			_, err := url.Parse(httpEndpoint)
			Expect(err).NotTo(HaveOccurred())

		})
	})
})
