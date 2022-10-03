package alerting

import (
	"context"

	"github.com/rancher/opni/pkg/alerting"
	"github.com/rancher/opni/pkg/alerting/backend"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"go.uber.org/zap"

	lru "github.com/hashicorp/golang-lru"

	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util/future"
)

const AlertingLogCacheSize = 32

type Plugin struct {
	alertingv1alpha.UnsafeAlertingServer
	system.UnimplementedSystemPluginClient
	Ctx             context.Context
	Logger          *zap.SugaredLogger
	inMemCache      *lru.Cache
	endpointBackend future.Future[backend.RuntimeEndpointBackend]
	AlertingOptions future.Future[shared.NewAlertingOptions]
	storage         future.Future[StorageAPIs]
	mgmtClient      future.Future[managementv1.ManagementClient]
	adminClient     future.Future[cortexadmin.CortexAdminClient]
}

type StorageAPIs struct {
	Conditions    storage.KeyValueStoreT[*alertingv1alpha.AlertCondition]
	AlertEndpoint storage.KeyValueStoreT[*alertingv1alpha.AlertEndpoint]
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewPluginLogger().Named("alerting")
	return &Plugin{
		Ctx:             ctx,
		Logger:          lg,
		inMemCache:      nil,
		mgmtClient:      future.New[managementv1.ManagementClient](),
		adminClient:     future.New[cortexadmin.CortexAdminClient](),
		endpointBackend: future.New[backend.RuntimeEndpointBackend](),
		AlertingOptions: future.New[shared.NewAlertingOptions](),
		storage:         future.New[StorageAPIs](),
	}
}

var _ alertingv1alpha.AlertingServer = (*Plugin)(nil)
var _ alerting.Provider = (alertingv1alpha.AlertingClient)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(managementext.ManagementAPIExtensionPluginID,
		managementext.NewPlugin(util.PackService(&alertingv1alpha.Alerting_ServiceDesc, p)))
	return scheme
}
