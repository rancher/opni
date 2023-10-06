package management

import (
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
	*driverutil.ContextKeyableConfigServer[
		*node.GetRequest,
		*node.SetRequest,
		*node.ResetRequest,
		*node.ConfigurationHistoryRequest,
		*node.ConfigurationHistoryResponse,
		*node.MetricsCapabilityConfig,
	]
}

func (m *NodeConfigService) Activate(ctx types.ManagementServiceContext) error {
	ctrl := ctx.RegisterService(util.PackService[node.NodeConfigurationServer](&node.NodeConfiguration_ServiceDesc, m))
	defer ctrl.SetServingStatus(managementext.Serving)

	defaultCapabilityStore := kvutil.WithKey(system.NewKVStoreClient[*node.MetricsCapabilityConfig](ctx.KeyValueStoreClient()), "/config/capability/default")
	activeCapabilityStore := kvutil.WithPrefix(system.NewKVStoreClient[*node.MetricsCapabilityConfig](ctx.KeyValueStoreClient()), "/config/capability/nodes/")

	m.ContextKeyableConfigServer = m.ContextKeyableConfigServer.New(defaultCapabilityStore, activeCapabilityStore, flagutil.LoadDefaults)

	return nil
}
