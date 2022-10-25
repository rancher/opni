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
	"github.com/rancher/opni/plugins/metrics/pkg/agent/drivers"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"go.uber.org/zap"
)

type Plugin struct {
	ctx    context.Context
	logger *zap.SugaredLogger

	httpServer   *HttpServer
	ruleStreamer *RuleStreamer
	node         *MetricsNode

	stopRuleStreamer context.CancelFunc
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewPluginLogger().Named("metrics")

	ct := healthpkg.NewDefaultConditionTracker(lg)

	p := &Plugin{
		ctx:          ctx,
		logger:       lg,
		httpServer:   NewHttpServer(ct, lg),
		ruleStreamer: NewRuleStreamer(ct, lg),
		node:         NewMetricsNode(ct, lg),
	}

	if d, err := drivers.NewExternalPromOperatorDriver(lg.Named("external-operator")); err != nil {
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

	p.node.AddConfigListener(drivers.NewListenerFunc(ctx, p.onConfigUpdated))

	return p
}

func (p *Plugin) onConfigUpdated(cfg *node.MetricsCapabilityConfig) {
	p.logger.Debug("metrics capability config updated")

	// at this point, we know the config has been updated
	currentlyRunning := (p.stopRuleStreamer != nil)
	shouldRun := cfg.GetEnabled()

	startRuleStreamer := func() {
		ctx, ca := context.WithCancel(p.ctx)
		p.stopRuleStreamer = ca
		go p.ruleStreamer.Run(ctx, cfg.GetSpec().GetRules())
	}

	switch {
	case currentlyRunning && shouldRun:
		p.logger.Debug("reconfiguring rule sync")
		p.stopRuleStreamer()
		startRuleStreamer()
	case currentlyRunning && !shouldRun:
		p.logger.Debug("stopping rule sync")
		p.stopRuleStreamer()
		p.stopRuleStreamer = nil
		p.logger.Debug("disabling http server")
		p.httpServer.SetEnabled(false)
	case !currentlyRunning && shouldRun:
		p.logger.Debug("starting rule sync")
		startRuleStreamer()
		p.logger.Debug("enabling http server")
		p.httpServer.SetEnabled(true)
	case !currentlyRunning && !shouldRun:
		p.logger.Debug("rule sync is disabled")
	}
}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeAgent))
	p := NewPlugin(ctx)
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewAgentPlugin(p.node))
	scheme.Add(health.HealthPluginID, health.NewPlugin(p.node))
	scheme.Add(stream.StreamAPIExtensionPluginID, stream.NewPlugin(p))
	scheme.Add(httpext.HTTPAPIExtensionPluginID, httpext.NewPlugin(p.httpServer))
	return scheme
}
