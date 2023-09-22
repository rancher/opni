package agent

import (
	"context"

	"log/slog"

	healthpkg "github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/health"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/topology/apis/node"
	"github.com/rancher/opni/plugins/topology/pkg/topology/agent/drivers"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Plugin struct {
	ctx    context.Context
	logger *slog.Logger

	node             *TopologyNode
	topologyStreamer *TopologyStreamer

	k8sClient future.Future[client.Client]

	stopStreaming context.CancelFunc
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewPluginLogger().WithGroup("topology")
	ct := healthpkg.NewDefaultConditionTracker(lg)
	p := &Plugin{
		ctx:              ctx,
		logger:           lg,
		node:             NewTopologyNode(ct, lg),
		topologyStreamer: NewTopologyStreamer(ct, lg),
		k8sClient:        future.New[client.Client](),
	}

	if d, err := drivers.NewExternalTopologyOperatorDriver(); err != nil {
		// doens't exist
		lg.With(
			"driver", d.Name(),
			logger.Err(err),
		).Info("node driver is unavailable")
		drivers.LogNodeDriverFailure(d.Name(), err)
	} else {
		lg.With(
			"driver", d.Name(),
		).Info("node driver is available")
		drivers.RegisterNodeDriver(d)
		p.node.AddConfigListener(drivers.NewListenerFunc(ctx, d.ConfigureNode))
	}

	p.node.AddConfigListener(drivers.NewListenerFunc(ctx, p.onConfigUpdated))

	return p
}

func (p *Plugin) onConfigUpdated(cfg *node.TopologyCapabilityConfig) {
	p.logger.Debug("topology capability config updated")

	// at this point we know the config has been updated
	currentlyRunning := (p.stopStreaming != nil)
	shouldRun := cfg.GetEnabled()

	startTopologyStream := func() {
		ctx, ca := context.WithCancel(p.ctx)
		p.stopStreaming = ca
		go p.topologyStreamer.Run(ctx, cfg.GetSpec())
	}

	switch {
	case currentlyRunning && shouldRun:
		p.logger.Debug("reconfiguring topology stream")
		p.stopStreaming()
		startTopologyStream()
	case currentlyRunning && !shouldRun:
		p.logger.Debug("stopping topology stream")
		p.stopStreaming()
		p.stopStreaming = nil
	case !currentlyRunning && shouldRun:
		p.logger.Debug("starting topology stream")
		startTopologyStream()
	case !currentlyRunning && !shouldRun:
		p.logger.Debug("topology streaming is disabled")
	}
}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeAgent))
	p := NewPlugin(ctx)
	scheme.Add(health.HealthPluginID, health.NewPlugin(p.node))
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewAgentPlugin(p.node))
	scheme.Add(stream.StreamAPIExtensionPluginID, stream.NewAgentPlugin(p))
	return scheme
}
