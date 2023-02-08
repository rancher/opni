package agent

import (
	"context"

	healthpkg "github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/health"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/plugins/logging/pkg/agent/drivers"
	"github.com/rancher/opni/plugins/logging/pkg/agent/drivers/events"
	"github.com/rancher/opni/plugins/logging/pkg/agent/drivers/kubernetes"
	loggingutil "github.com/rancher/opni/plugins/logging/pkg/util"
	"go.uber.org/zap"
)

type Plugin struct {
	ctx           context.Context
	logger        *zap.SugaredLogger
	node          *LoggingNode
	otelForwarder *loggingutil.OTELForwarder
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewPluginLogger().Named("logging")

	ct := healthpkg.NewDefaultConditionTracker(lg)

	p := &Plugin{
		ctx:    ctx,
		logger: lg,
		node:   NewLoggingNode(ct, lg),
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

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeAgent))
	p := NewPlugin(ctx)
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewAgentPlugin(p.node))
	scheme.Add(health.HealthPluginID, health.NewPlugin(p.node))
	scheme.Add(stream.StreamAPIExtensionPluginID, stream.NewPlugin(p))
	return scheme
}
