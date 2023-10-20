package agent

import (
	"context"
	"fmt"

	"log/slog"

	healthpkg "github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/logger"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/health"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/util/notifier"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/rancher/opni/plugins/metrics/pkg/agent/drivers"
)

type Plugin struct {
	ctx    context.Context
	logger *slog.Logger

	httpServer   *HttpServer
	ruleStreamer *RuleStreamer
	node         *MetricsNode

	stopRuleStreamer context.CancelFunc
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewPluginLogger().WithGroup("metrics")

	ct := healthpkg.NewDefaultConditionTracker(lg)

	p := &Plugin{
		ctx:          ctx,
		logger:       lg,
		httpServer:   NewHttpServer(ct, lg),
		ruleStreamer: NewRuleStreamer(ct, lg),
		node:         NewMetricsNode(ct, lg),
	}

	for _, name := range drivers.NodeDrivers.List() {
		builder, ok := drivers.NodeDrivers.Get(name)
		if !ok {
			continue
		}
		driver, err := builder(ctx)
		if err != nil {
			lg.With(
				"driver", name,
				zap.Error(err),
			).Warn("failed to initialize node driver")
			continue
		}
		p.node.AddConfigListener(driver)
		p.node.AddNodeDriver(driver)
	}

	p.node.AddConfigListener(p)
	return p
}

func (p *Plugin) ConfigureNode(nodeId string, cfg *node.MetricsCapabilityConfig) error {
	lg := p.logger.With("nodeId", nodeId)
	lg.Debug("metrics capability config updated")

	// at this point, we know the config has been updated
	currentlyRunning := (p.stopRuleStreamer != nil)
	shouldRun := cfg.GetEnabled()

	startRuleStreamer := func() {
		ctx, ca := context.WithCancel(p.ctx)
		p.stopRuleStreamer = ca
		finders := []notifier.Finder[rules.RuleGroup]{}
		for name, driver := range p.node.nodeDrivers {
			if !cfg.Spec.RuleDiscoveryEnabled() {
				continue
			}
			if f := driver.ConfigureRuleGroupFinder(cfg.Spec.Rules); f != nil {
				lg.Info(fmt.Sprintf("prometheus rule finder configured for driver %d", name))
				finders = append(finders, f)
			}
		}
		go p.ruleStreamer.Run(ctx, cfg.GetSpec().GetRules(), notifier.NewMultiFinder(finders...))
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

	return nil
}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeAgent))
	p := NewPlugin(ctx)
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewAgentPlugin(p.node))
	scheme.Add(health.HealthPluginID, health.NewPlugin(p.node))
	scheme.Add(stream.StreamAPIExtensionPluginID, stream.NewAgentPlugin(p))
	scheme.Add(httpext.HTTPAPIExtensionPluginID, httpext.NewPlugin(p.httpServer))
	return scheme
}
