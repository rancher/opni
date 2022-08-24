package slo

import (
	"context"

	"go.uber.org/zap"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
)

type Plugin struct {
	sloapi.UnsafeSLOServer
	system.UnimplementedSystemPluginClient
	ctx         context.Context
	logger      *zap.SugaredLogger
	storage     future.Future[StorageAPIs]
	mgmtClient  future.Future[managementv1.ManagementClient]
	adminClient future.Future[cortexadmin.CortexAdminClient]
}

type StorageAPIs struct {
	SLOs     storage.KeyValueStoreT[*sloapi.SLOData]
	Services storage.KeyValueStoreT[*sloapi.Service]
	Metrics  storage.KeyValueStoreT[*sloapi.Metric]
}

func NewPlugin(ctx context.Context) *Plugin {
	return &Plugin{
		ctx:         ctx,
		logger:      logger.NewPluginLogger().Named("slo"),
		storage:     future.New[StorageAPIs](),
		mgmtClient:  future.New[managementv1.ManagementClient](),
		adminClient: future.New[cortexadmin.CortexAdminClient](),
	}
}

var _ sloapi.SLOServer = (*Plugin)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(managementext.ManagementAPIExtensionPluginID,
		managementext.NewPlugin(util.PackService(&sloapi.SLO_ServiceDesc, p)))
	return scheme
}
