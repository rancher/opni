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
	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/util/notifier"
	"github.com/rancher/opni/plugins/metrics/pkg/agent/drivers"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"github.com/rancher/opni/plugins/metrics/pkg/otel"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Plugin struct {
	ctx    context.Context
	logger *zap.SugaredLogger

	otelForwarder otel.OTELForwarder
	httpServer    *HttpServer
	ruleStreamer  *RuleStreamer
	node          *MetricsNode

	stopRuleStreamer context.CancelFunc
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewPluginLogger().Named("metrics")

	ct := healthpkg.NewDefaultConditionTracker(lg)

	otelForwarder := otel.NewOTELForwarder(
		ctx,
		otel.WithPrivileged(true),
		otel.WithDialOptions(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)

	p := &Plugin{
		ctx:    ctx,
		logger: lg,
		httpServer: NewHttpServer(ct, lg,
			otelForwarder.Routes(),
		),
		ruleStreamer:  NewRuleStreamer(ct, lg),
		node:          NewMetricsNode(ct, lg),
		otelForwarder: otelForwarder,
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
		p.node.AddConfigListener(drivers.NewListenerFunc(ctx, driver.ConfigureNode))
		p.node.AddNodeDriver(driver)
	}

	p.node.AddConfigListener(drivers.NewListenerFunc(ctx, p.onConfigUpdated))
	return p
}

func (p *Plugin) onConfigUpdated(nodeId string, cfg *node.MetricsCapabilityConfig) {
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
				lg.Infof("prometheus rule finder configured for driver %s", name)
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
