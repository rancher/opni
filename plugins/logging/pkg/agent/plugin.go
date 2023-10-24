package agent

import (
	"context"

	"log/slog"

	healthpkg "github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/logger"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/health"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/plugins/logging/pkg/agent/drivers"
	"github.com/rancher/opni/plugins/logging/pkg/otel"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
)

type Plugin struct {
	ctx           context.Context
	logger        *slog.Logger
	node          *LoggingNode
	otelForwarder *otel.OTELForwarder
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewPluginLogger().WithGroup("logging")

	ct := healthpkg.NewDefaultConditionTracker(lg)

	p := &Plugin{
		ctx:           ctx,
		logger:        lg,
		node:          NewLoggingNode(ct, lg),
		otelForwarder: otel.NewOTELForwarder(otel.WithLogger(lg.WithGroup("otel-forwarder"))),
	}

	for _, d := range drivers.NodeDrivers.List() {
		driverBuilder, _ := drivers.NodeDrivers.Get(d)
		driver, err := driverBuilder(ctx,
			driverutil.NewOption("logger", lg),
		)
		if err != nil {
			lg.With(
				"driver", d,
				logger.Err(err),
			).Error("failed to initialize logging node driver")
			continue
		}

		p.node.AddConfigListener(drivers.NewListenerFunc(ctx, driver.ConfigureNode))
	}
	return p
}

var _ collogspb.LogsServiceServer = (*otel.OTELForwarder)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeAgent))
	p := NewPlugin(ctx)
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewAgentPlugin(p.node))
	scheme.Add(health.HealthPluginID, health.NewPlugin(p.node))
	scheme.Add(stream.StreamAPIExtensionPluginID, stream.NewAgentPlugin(p))
	scheme.Add(httpext.HTTPAPIExtensionPluginID, httpext.NewPlugin(p.otelForwarder))
	return scheme
}
