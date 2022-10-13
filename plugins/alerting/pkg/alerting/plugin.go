package alerting

import (
	"context"

	"github.com/rancher/opni/pkg/alerting/backend"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"go.uber.org/zap"

	lru "github.com/hashicorp/golang-lru"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util/future"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/condition"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/endpoint"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/log"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/trigger"
)

const AlertingLogCacheSize = 32

type Plugin struct {
	condition.UnsafeAlertingConditionsServer
	endpoint.UnsafeAlertingEndpointsServer
	log.UnsafeAlertingLogsServer
	trigger.UnsafeAlertingServer

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

var _ endpoint.AlertingEndpointsServer = (*Plugin)(nil)
var _ condition.AlertingConditionsServer = (*Plugin)(nil)
var _ log.AlertingLogsServer = (*Plugin)(nil)
var _ trigger.AlertingServer = (*Plugin)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	// We omit the trigger server here since it should only be used internally
	scheme.Add(managementext.ManagementAPIExtensionPluginID,
		managementext.NewPlugin(
			util.PackService(
				&condition.AlertingConditions_ServiceDesc,
				p,
			),
			util.PackService(
				&endpoint.AlertingEndpoints_ServiceDesc,
				p,
			),
			util.PackService(
				&log.AlertingLogs_ServiceDesc,
				p,
			),
		),
	)
	return scheme
}
