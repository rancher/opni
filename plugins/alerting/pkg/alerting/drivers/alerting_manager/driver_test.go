package alerting_manager_test

import (
	"context"
	"crypto/tls"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers/alerting_manager"
)

var _ = Describe("", Label("unit"), func() {
	When("We register the alering cluster driver", func() {
		It("should apply the tls config via driver options", func() {
			tlsConfig := &tls.Config{}
			opts := []driverutil.Option{
				driverutil.NewOption("tlsConfig", tlsConfig),
			}

			ctx := context.Background()
			lg := logger.NewPluginLogger(ctx).WithGroup("alerting").WithGroup("alerting-manager")

			options := alerting_manager.AlertingDriverOptions{
				ConfigKey:          shared.AlertManagerConfigKey,
				InternalRoutingKey: shared.InternalRoutingConfigKey,
				Context:            logger.WithPluginLogger(ctx, lg),
			}
			driverutil.ApplyOptions(&options, opts...)
			Expect(options.TlsConfig).NotTo(BeNil())
		})

		It("should apply cluster driver subscribers via driver options", func() {
			subscriberA := make(chan client.AlertingClient)
			subscriberB := make(chan client.AlertingClient)
			opts := []driverutil.Option{
				driverutil.NewOption("subscribers", []chan client.AlertingClient{subscriberA, subscriberB}),
			}
			ctx := context.Background()
			lg := logger.NewPluginLogger(ctx).WithGroup("alerting").WithGroup("alerting-manager")

			options := alerting_manager.AlertingDriverOptions{
				ConfigKey:          shared.AlertManagerConfigKey,
				InternalRoutingKey: shared.InternalRoutingConfigKey,
				Context:            logger.WithPluginLogger(ctx, lg),
			}
			driverutil.ApplyOptions(&options, opts...)
			Expect(options.Subscribers).To(HaveLen(2))
		})
	})
})
