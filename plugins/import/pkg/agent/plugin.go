package agent

import (
	"context"
	healthpkg "github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/health"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/plugins/import/pkg/agent/drivers"
	"go.uber.org/zap"
)

type Plugin struct {
	ctx    context.Context
	logger *zap.SugaredLogger

	node *ImportNode
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewPluginLogger().Named("import")
	ct := healthpkg.NewDefaultConditionTracker(lg)

	p := &Plugin{
		ctx:    ctx,
		logger: lg,
		node:   NewImportNode(ct, lg),
	}

	if d, err := drivers.NewExternalOperatorDriver(lg.Named(drivers.ExternalOperatorOperatorDriverName)); err != nil {
		lg.With(
			"driver", d.Name(),
			zap.Error(err),
		).Info("node driver is unavailable")
		drivers.LogNodeDriverFailure(d.Name(), err)
	} else {
		lg.With(
			"driver", d.Name(),
		).Infof("node driver is available")
		drivers.RegisterNodeDriver(d)
		p.node.SetNodeDriver(d)
	}

	return p
}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeAgent))
	p := NewPlugin(ctx)

	scheme.Add(health.HealthPluginID, health.NewPlugin(p.node))
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewAgentPlugin(p.node))
	scheme.Add(stream.StreamAPIExtensionPluginID, stream.NewPlugin(p))

	return scheme
}
