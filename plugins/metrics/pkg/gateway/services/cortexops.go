package services

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
	Context types.ManagementServiceContext `option:"context"`
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

func (s *CortexOpsService) Activate() error {
	defer s.Context.SetServingStatus(cortexops.CortexOps_ServiceDesc.ServiceName, managementext.Serving)

	defaultStore := kvutil.WithKey(system.NewKVStoreClient[*cortexops.CapabilityBackendConfigSpec](s.Context.KeyValueStoreClient()), "/config/cluster/default")
	activeStore := s.Context.ClusterDriver().ActiveConfigStore()

	s.BaseConfigServer = s.BaseConfigServer.Build(defaultStore, activeStore, flagutil.LoadDefaults)
	s.PartialCortexOpsServer = s.Context.ClusterDriver()
	return nil
}

func (s *CortexOpsService) ManagementServices() []util.ServicePackInterface {
	return []util.ServicePackInterface{
		util.PackService[cortexops.CortexOpsServer](&cortexops.CortexOps_ServiceDesc, s),
	}
}

func (s *CortexOpsService) DryRun(ctx context.Context, req *cortexops.DryRunRequest) (*cortexops.DryRunResponse, error) {
	res, err := s.Tracker().DryRun(ctx, req)
	if err != nil {
		return nil, err
	}
	return &cortexops.DryRunResponse{
		Current:          res.Current,
		Modified:         res.Modified,
		ValidationErrors: configutil.ValidateConfiguration(res.Modified),
	}, nil
}

func init() {
	types.Services.Register("Cortex Ops Service", func(_ context.Context, opts ...driverutil.Option) (types.Service, error) {
		svc := &CortexOpsService{}
		driverutil.ApplyOptions(svc, opts...)
		return svc, nil
	})
}
