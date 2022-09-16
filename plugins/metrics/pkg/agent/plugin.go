package agent

import (
	"context"

	"github.com/rancher/opni/pkg/logger"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/health"
	"github.com/rancher/opni/pkg/plugins/meta"
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

	ct := NewConditionTracker(lg)

	p := &Plugin{
		ctx:          ctx,
		logger:       lg,
		httpServer:   NewHttpServer(ct, lg),
		ruleStreamer: NewRuleStreamer(ct, lg),
		node:         NewMetricsNode(ct, lg),
	}

	listenerC := make(chan *node.MetricsCapabilityConfig, 1)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case cfg := <-listenerC:
				p.onConfigUpdated(cfg)
			}
		}
	}()
	p.node.AddConfigListener(listenerC)

	return p
}

func (p *Plugin) onConfigUpdated(cfg *node.MetricsCapabilityConfig) {
	p.logger.Debugf("metrics capability config updated: %v", cfg)

	if p.stopRuleStreamer != nil {
		p.logger.Debug("reconfiguring rule sync")
		p.stopRuleStreamer()
	} else {
		p.logger.Debug("starting rule sync")
	}
	ctx, ca := context.WithCancel(p.ctx)
	p.stopRuleStreamer = ca
	go p.ruleStreamer.Run(ctx, cfg.Rules)
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
