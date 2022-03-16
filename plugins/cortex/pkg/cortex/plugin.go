package cortex

import (
	"context"

	"github.com/cortexproject/cortex/pkg/ingester/client"
	"github.com/hashicorp/go-hclog"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/util"
	"github.com/rancher/opni-monitoring/plugins/cortex/pkg/apis/cortexadmin"
)

type Plugin struct {
	cortexadmin.UnimplementedCortexAdminServer
	ctx               context.Context
	config            *util.Future[*v1beta1.GatewayConfig]
	mgmtApi           *util.Future[management.ManagementClient]
	storageBackend    *util.Future[storage.Backend]
	distributorClient *util.Future[client.IngesterClient]
	logger            hclog.Logger
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewForPlugin()
	lg.SetLevel(hclog.Debug)
	return &Plugin{
		ctx:               ctx,
		config:            util.NewFuture[*v1beta1.GatewayConfig](),
		mgmtApi:           util.NewFuture[management.ManagementClient](),
		storageBackend:    util.NewFuture[storage.Backend](),
		distributorClient: util.NewFuture[client.IngesterClient](),
		logger:            lg,
	}
}

var _ cortexadmin.CortexAdminServer = (*Plugin)(nil)
