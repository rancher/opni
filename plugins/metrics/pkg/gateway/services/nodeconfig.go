package services

import (
	"context"

	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/storage/kvutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/flagutil"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/rancher/opni/plugins/metrics/pkg/types"
)

type NodeConfigService struct {
	Context types.ManagementServiceContext `option:"context"`
	*driverutil.ContextKeyableConfigServer[
		*node.GetRequest,
		*node.SetRequest,
		*node.ResetRequest,
		*node.ConfigurationHistoryRequest,
		*node.ConfigurationHistoryResponse,
		*node.MetricsCapabilityConfig,
	]
}

func (s *NodeConfigService) Activate() error {
	defer s.Context.SetServingStatus(node.NodeConfiguration_ServiceDesc.ServiceName, managementext.Serving)

	defaultCapabilityStore := kvutil.WithKey(system.NewKVStoreClient[*node.MetricsCapabilityConfig](s.Context.KeyValueStoreClient()), "/config/capability/default")
	activeCapabilityStore := kvutil.WithPrefix(system.NewKVStoreClient[*node.MetricsCapabilityConfig](s.Context.KeyValueStoreClient()), "/config/capability/nodes/")

	s.ContextKeyableConfigServer = s.ContextKeyableConfigServer.Build(defaultCapabilityStore, activeCapabilityStore, flagutil.LoadDefaults)
	StartActiveSyncWatcher(s.Context, activeCapabilityStore)
	StartDefaultSyncWatcher(s.Context, defaultCapabilityStore)

	return nil
}

func (s *NodeConfigService) ManagementServices() []util.ServicePackInterface {
	return []util.ServicePackInterface{
		util.PackService[node.NodeConfigurationServer](&node.NodeConfiguration_ServiceDesc, s),
	}
}

// DryRun implements node.NodeConfigurationServer.
func (s *NodeConfigService) DryRun(ctx context.Context, req *node.DryRunRequest) (*node.DryRunResponse, error) {
	res, err := s.Tracker().DryRun(ctx, req)
	if err != nil {
		return nil, err
	}
	return &node.DryRunResponse{
		Current:          res.Current,
		Modified:         res.Modified,
		ValidationErrors: []*driverutil.ValidationError{}, // TODO: implement node config validation
	}, nil
}

func init() {
	types.Services.Register("Node Config Service", func(_ context.Context, opts ...driverutil.Option) (types.Service, error) {
		svc := &NodeConfigService{}
		driverutil.ApplyOptions(svc, opts...)
		return svc, nil
	})
}
