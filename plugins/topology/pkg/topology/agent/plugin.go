package agent

import (
	"context"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/topology/pkg/apis/remote"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Plugin struct {
	ctx    context.Context
	logger *zap.SugaredLogger
	remote.UnsafeRemoteTopologyServer
	system.UnimplementedSystemPluginClient

	mgmtClient future.Future[managementv1.ManagementClient]
	k8sClient  future.Future[client.Client]
}

func NewPlugin(ctx context.Context) *Plugin {
	return &Plugin{
		ctx:    ctx,
		logger: logger.NewPluginLogger().Named("topology"),
	}
}

var _ remote.RemoteTopologyServer = (*Plugin)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeAgent))
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	// scheme.Add(stream.StreamAPIExtensionPluginID, stream.NewPlugin(p))
	return scheme
}
