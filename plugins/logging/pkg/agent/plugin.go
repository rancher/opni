package agent

import (
	"context"

	healthpkg "github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/logger"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/health"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/plugins/logging/pkg/agent/drivers"
	"github.com/rancher/opni/plugins/logging/pkg/agent/drivers/events"
	"github.com/rancher/opni/plugins/logging/pkg/agent/drivers/kubernetes"
	"github.com/rancher/opni/plugins/logging/pkg/otel"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"go.uber.org/zap"
)

type Plugin struct {
	ctx           context.Context
	logger        *zap.SugaredLogger
	node          *LoggingNode
	otelForwarder *otel.OTELForwarder
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewPluginLogger().Named("logging")

	ct := healthpkg.NewDefaultConditionTracker(lg)

	p := &Plugin{
		ctx:           ctx,
		logger:        lg,
		node:          NewLoggingNode(ct, lg),
		otelForwarder: otel.NewOTELForwarder(otel.WithLogger(lg.Named("otel-forwarder"))),
	}

	if d, err := kubernetes.NewKubernetesManagerDriver(lg.Named("kubernetes-manager")); err != nil {
		lg.With(
			"driver", d.Name(),
			zap.Error(err),
		).Info("node driver is unavailable")
		drivers.LogNodeDriverFailure(d.Name(), err)
	} else {
		lg.With(
			"driver", d.Name(),
		).Info("node driver is available")
		drivers.RegisterNodeDriver(d)
		p.node.AddConfigListener(drivers.NewListenerFunc(ctx, d.ConfigureNode))
	}

	if c, err := events.NewEventCollector(lg.Named("event-collector")); err != nil {
		lg.With(
			"driver", c.Name(),
			zap.Error(err),
		).Info("node driver is unavailable")
		drivers.LogNodeDriverFailure(c.Name(), err)
	} else {
		lg.With(
			"driver", c.Name(),
		).Info("node driver is available")
		drivers.RegisterNodeDriver(c)
		p.node.AddConfigListener(drivers.NewListenerFunc(ctx, c.ConfigureNode))
	}

	return p
}

var _ collogspb.LogsServiceServer = (*otel.OTELForwarder)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeAgent))
	p := NewPlugin(ctx)
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewAgentPlugin(p.node))
	scheme.Add(health.HealthPluginID, health.NewPlugin(p.node))
	scheme.Add(stream.StreamAPIExtensionPluginID, stream.NewPlugin(p))
	scheme.Add(httpext.HTTPAPIExtensionPluginID, httpext.NewPlugin(p.otelForwarder))
	return scheme
}
