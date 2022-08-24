package cortex

import (
	"context"
	"crypto/tls"
	"net/http"

	"github.com/cortexproject/cortex/pkg/distributor/distributorpb"
	"go.uber.org/zap"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/metrics/collector"
	gatewayext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/gateway"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/gateway/stream"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/metrics"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexops"
)

type Plugin struct {
	cortexadmin.UnsafeCortexAdminServer
	cortexops.UnsafeCortexOpsServer
	capabilityv1.UnsafeBackendServer
	system.UnimplementedSystemPluginClient
	collector.CollectorServer
	ctx                 context.Context
	config              future.Future[*v1beta1.GatewayConfig]
	authMiddlewares     future.Future[map[string]auth.Middleware]
	mgmtApi             future.Future[managementv1.ManagementClient]
	storageBackend      future.Future[storage.Backend]
	distributorClient   future.Future[distributorpb.DistributorClient]
	cortexHttpClient    future.Future[*http.Client]
	cortexTlsConfig     future.Future[*tls.Config]
	uninstallController future.Future[*task.Controller]
	logger              *zap.SugaredLogger
}

func NewPlugin(ctx context.Context) *Plugin {
	return &Plugin{
		CollectorServer:     collectorServer,
		ctx:                 ctx,
		config:              future.New[*v1beta1.GatewayConfig](),
		authMiddlewares:     future.New[map[string]auth.Middleware](),
		mgmtApi:             future.New[managementv1.ManagementClient](),
		storageBackend:      future.New[storage.Backend](),
		distributorClient:   future.New[distributorpb.DistributorClient](),
		cortexHttpClient:    future.New[*http.Client](),
		cortexTlsConfig:     future.New[*tls.Config](),
		uninstallController: future.New[*task.Controller](),
		logger:              logger.NewPluginLogger().Named("cortex"),
	}
}

var _ cortexadmin.CortexAdminServer = (*Plugin)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(gatewayext.GatewayAPIExtensionPluginID, gatewayext.NewPlugin(p))
	scheme.Add(streamext.StreamAPIExtensionPluginID, streamext.NewPlugin(p))
	scheme.Add(managementext.ManagementAPIExtensionPluginID, managementext.NewPlugin(
		util.PackService(&cortexadmin.CortexAdmin_ServiceDesc, p),
		util.PackService(&cortexops.CortexOps_ServiceDesc, p),
	))
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewPlugin(p))
	scheme.Add(metrics.MetricsPluginID, metrics.NewPlugin(p))
	return scheme
}
