package gateway

import (
	"context"

	"github.com/nats-io/nats.go"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/topology/pkg/apis/orchestrator"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Plugin struct {
	orchestrator.UnsafeTopologyOrchestratorServer
	system.UnimplementedSystemPluginClient

	ctx    context.Context
	logger *zap.SugaredLogger

	nc      future.Future[*nats.Conn]
	storage future.Future[ConfigStorageAPIs]

	mgmtClient future.Future[managementv1.ManagementClient]
	k8sClient  future.Future[client.Client]
}

func NewPlugin(ctx context.Context) *Plugin {
	return &Plugin{
		ctx:        ctx,
		logger:     logger.NewPluginLogger().Named("topology"),
		nc:         future.New[*nats.Conn](),
		storage:    future.New[ConfigStorageAPIs](),
		mgmtClient: future.New[managementv1.ManagementClient](),
		k8sClient:  future.New[client.Client](),
	}
}

var _ orchestrator.TopologyOrchestratorServer = (*Plugin)(nil)

type ConfigStorageAPIs struct {
	//FIXME: to rename if we find a use
	Placeholder storage.KeyValueStoreT[proto.Message]
}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeGateway))
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(managementext.ManagementAPIExtensionPluginID,
		managementext.NewPlugin(util.PackService(&orchestrator.TopologyOrchestrator_ServiceDesc, p)))
	return scheme
}
