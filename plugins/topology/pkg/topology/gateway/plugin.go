package gateway

import (
	"context"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/topology/pkg/apis/orchestrator"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Plugin struct {
	ctx    context.Context
	logger *zap.SugaredLogger
	orchestrator.UnsafeTopologyOrchestratorServer
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

var _ orchestrator.TopologyOrchestratorServer = (*Plugin)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeGateway))
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(managementext.ManagementAPIExtensionPluginID,
		managementext.NewPlugin(util.PackService(&orchestrator.TopologyOrchestrator_ServiceDesc, p)))
	return scheme
}
