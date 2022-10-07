package gateway

/*
Declares the topology plugin's gateway meta scheme.
*/

import (
	"context"

	"github.com/nats-io/nats.go"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/topology/pkg/apis/orchestrator"
	"github.com/rancher/opni/plugins/topology/pkg/backend"
	"github.com/rancher/opni/plugins/topology/pkg/topology/gateway/drivers"
	"github.com/rancher/opni/plugins/topology/pkg/topology/gateway/stream"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Plugin struct {
	orchestrator.UnsafeTopologyOrchestratorServer
	system.UnimplementedSystemPluginClient

	ctx    context.Context
	logger *zap.SugaredLogger

	topologyRemoteWrite stream.TopologyRemoteWriter
	topologyBackend     backend.TopologyBackend

	nc      future.Future[*nats.Conn]
	storage future.Future[ConfigStorageAPIs]

	mgmtClient future.Future[managementv1.ManagementClient]
	// remove this if we decide to acquire the opni-manager cluster driver
	k8sClient           future.Future[client.Client]
	storageBackend      future.Future[storage.Backend]
	nodeManagerClient   future.Future[capabilityv1.NodeManagerClient]
	uninstallController future.Future[*task.Controller]
	clusterDriver       future.Future[drivers.ClusterDriver]
}

func NewPlugin(ctx context.Context) *Plugin {
	p := &Plugin{
		ctx:        ctx,
		logger:     logger.NewPluginLogger().Named("topology"),
		nc:         future.New[*nats.Conn](),
		storage:    future.New[ConfigStorageAPIs](),
		mgmtClient: future.New[managementv1.ManagementClient](),
		k8sClient:  future.New[client.Client](),
	}

	p.topologyRemoteWrite.Initialize(stream.TopologyRemoteWriteConfig{})
	p.logger.Debug("Waiting for async requirements for starting topology backend")
	future.Wait5(p.storageBackend, p.mgmtClient, p.nodeManagerClient, p.uninstallController, p.clusterDriver,
		func(
			storageBackend storage.Backend,
			mgmtClient managementv1.ManagementClient,
			nodeManagerClient capabilityv1.NodeManagerClient,
			uninstallController *task.Controller,
			clusterDriver drivers.ClusterDriver,
		) {
			p.topologyBackend.Initialize(backend.TopologyBackendConfig{
				Logger:              p.logger.Named("topology-backend"),
				StorageBackend:      storageBackend,
				MgmtClient:          mgmtClient,
				NodeManagerClient:   nodeManagerClient,
				UninstallController: uninstallController,
				ClusterDriver:       clusterDriver,
			})
		})
	p.logger.Debug("Initialized topology backend")

	return p
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
		managementext.NewPlugin(
			util.PackService(
				&orchestrator.TopologyOrchestrator_ServiceDesc,
				p,
			),
		),
	)
	scheme.Add(streamext.StreamAPIExtensionPluginID, streamext.NewPlugin(p))
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewPlugin(&p.topologyBackend))
	return scheme
}
