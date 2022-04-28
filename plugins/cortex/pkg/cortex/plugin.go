package cortex

import (
	"context"
	"net/http"

	"github.com/cortexproject/cortex/pkg/distributor/distributorpb"
	ingesterclient "github.com/cortexproject/cortex/pkg/ingester/client"
	"github.com/hashicorp/go-hclog"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/management"
	"github.com/rancher/opni/pkg/metrics/collector"
	gatewayext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/gateway"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/metrics"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
)

type Plugin struct {
	cortexadmin.UnsafeCortexAdminServer
	collector.CollectorServer
	ctx               context.Context
	config            *util.Future[*v1beta1.GatewayConfig]
	mgmtApi           *util.Future[management.ManagementClient]
	storageBackend    *util.Future[storage.Backend]
	distributorClient *util.Future[distributorpb.DistributorClient]
	ingesterClient    *util.Future[ingesterclient.IngesterClient]
	cortexHttpClient  *util.Future[http.Client]
	logger            hclog.Logger
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewForPlugin()
	lg.SetLevel(hclog.Debug)
	return &Plugin{
		CollectorServer:   collectorServer,
		ctx:               ctx,
		config:            util.NewFuture[*v1beta1.GatewayConfig](),
		mgmtApi:           util.NewFuture[management.ManagementClient](),
		storageBackend:    util.NewFuture[storage.Backend](),
		distributorClient: util.NewFuture[distributorpb.DistributorClient](),
		ingesterClient:    util.NewFuture[ingesterclient.IngesterClient](),
		cortexHttpClient:  util.NewFuture[http.Client](),
		logger:            lg,
	}
}

var _ cortexadmin.CortexAdminServer = (*Plugin)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(gatewayext.GatewayAPIExtensionPluginID, gatewayext.NewPlugin(p))
	scheme.Add(managementext.ManagementAPIExtensionPluginID,
		managementext.NewPlugin(&cortexadmin.CortexAdmin_ServiceDesc, p))
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewPlugin(wellknown.CapabilityMetrics, p))
	scheme.Add(metrics.MetricsPluginID, metrics.NewPlugin(p))
	return scheme
}
