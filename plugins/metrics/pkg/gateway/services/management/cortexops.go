package management

import (
	"context"

	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/storage/kvutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/flagutil"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"github.com/rancher/opni/plugins/metrics/pkg/cortex/configutil"
	"github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"
	"github.com/rancher/opni/plugins/metrics/pkg/types"
)

type CortexOpsService struct {
	*driverutil.BaseConfigServer[
		*driverutil.GetRequest,
		*cortexops.SetRequest,
		*cortexops.ResetRequest,
		*driverutil.ConfigurationHistoryRequest,
		*cortexops.ConfigurationHistoryResponse,
		*cortexops.CapabilityBackendConfigSpec,
	]
	drivers.PartialCortexOpsServer
}

var _ cortexops.CortexOpsServer = (*CortexOpsService)(nil)

func (m *CortexOpsService) Activate(ctx types.ManagementServiceContext) error {
	ctrl := ctx.RegisterService(util.PackService[cortexops.CortexOpsServer](&cortexops.CortexOps_ServiceDesc, m))
	defer ctrl.SetServingStatus(managementext.Serving)

	defaultStore := kvutil.WithKey(system.NewKVStoreClient[*cortexops.CapabilityBackendConfigSpec](ctx.KeyValueStoreClient()), "/config/cluster/default")
	activeStore := ctx.ClusterDriver().ActiveConfigStore()

	m.BaseConfigServer = m.BaseConfigServer.New(defaultStore, activeStore, flagutil.LoadDefaults)
	m.PartialCortexOpsServer = ctx.ClusterDriver()
	return nil
}

func (k *CortexOpsService) DryRun(ctx context.Context, req *cortexops.DryRunRequest) (*cortexops.DryRunResponse, error) {
	res, err := k.Tracker().DryRun(ctx, req)
	if err != nil {
		return nil, err
	}
	return &cortexops.DryRunResponse{
		Current:          res.Current,
		Modified:         res.Modified,
		ValidationErrors: configutil.ValidateConfiguration(res.Modified),
	}, nil
}
