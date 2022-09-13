package agent

import (
	"context"

	"github.com/rancher/opni/pkg/logger"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/health"
	"github.com/rancher/opni/pkg/plugins/meta"
	"go.uber.org/zap"
)

type Plugin struct {
	ctx    context.Context
	logger *zap.SugaredLogger

	httpServer   *HttpServer
	ruleStreamer *RuleStreamer
	node         *MetricsNode
}

func NewPlugin(ctx context.Context) *Plugin {
	return &Plugin{
		ctx:          ctx,
		logger:       logger.NewPluginLogger().Named("cortex"),
		httpServer:   &HttpServer{},
		ruleStreamer: &RuleStreamer{},
		node:         &MetricsNode{},
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
