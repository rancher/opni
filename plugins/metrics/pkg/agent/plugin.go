package agent

import (
	"context"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/logger"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/health"
	"github.com/rancher/opni/pkg/plugins/meta"
	"go.uber.org/zap"
)

type Plugin struct {
	capabilityv1.UnsafeNodeServer
	controlv1.UnsafeHealthServer
	ctx    context.Context
	logger *zap.SugaredLogger

	httpServer *HttpServer
}

func NewPlugin(ctx context.Context) *Plugin {
	return &Plugin{
		ctx:        ctx,
		logger:     logger.NewPluginLogger().Named("cortex"),
		httpServer: &HttpServer{},
	}
}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeAgent))
	p := NewPlugin(ctx)
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewAgentPlugin(p))
	scheme.Add(health.HealthPluginID, health.NewPlugin(p))
	scheme.Add(stream.StreamAPIExtensionPluginID, stream.NewPlugin(p))
	scheme.Add(httpext.HTTPAPIExtensionPluginID, httpext.NewPlugin(p.httpServer))
	return scheme
}
