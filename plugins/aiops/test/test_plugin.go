package test

import (
	"context"
	"fmt"

	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/aiops/pkg/gateway"
)

func init() {
	test.EnablePlugin(meta.ModeGateway, func(ctx context.Context) meta.Scheme {
		env := test.EnvFromContext(ctx)
		port := env.GetPorts().MetricAnomalyServicePort
		return gateway.SchemeWith(ctx,
			driverutil.NewOption("metricAnomalyServiceURL", fmt.Sprintf("http://localhost:%d", port)),
		)
	})
}
