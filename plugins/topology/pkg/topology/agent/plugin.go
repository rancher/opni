package agent

import (
	"context"

	healthpkg "github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"github.com/rancher/opni/plugins/topology/pkg/apis/remote"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Plugin struct {
	ctx    context.Context
	logger *zap.SugaredLogger

	remote.UnsafeRemoteTopologyServer
	node *TopologyNode

	system.UnimplementedSystemPluginClient // FIXME: maybe don't need this
	k8sClient                              future.Future[client.Client]
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewPluginLogger().Named("topology")
	ct := healthpkg.NewDefaultConditionTracker(lg)
	p := &Plugin{
		ctx:    ctx,
		logger: lg,
		node:   NewTopologyNode(ct, lg),
	}

	// TODO : do the drivers thing here
	return p
}

func (p *Plugin) onConfigUpdated(cfg *node.MetricsCapabilityConfig) {
	p.logger.Debug("topology capability config updated")

	// at this point we know the config has been updated
}

var _ remote.RemoteTopologyServer = (*Plugin)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeAgent))
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(stream.StreamAPIExtensionPluginID, stream.NewPlugin(p))
	return scheme
}
