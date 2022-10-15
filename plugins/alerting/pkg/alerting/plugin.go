package alerting

import (
	"context"

	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/ops"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"go.uber.org/zap"

	lru "github.com/hashicorp/golang-lru"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alertstorage"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/condition"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/endpoint"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/log"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/trigger"
)

const AlertingLogCacheSize = 32

type Plugin struct {
	system.UnimplementedSystemPluginClient
	condition.UnsafeAlertConditionsServer
	endpoint.UnsafeAlertEndpointsServer
	log.UnsafeAlertLogsServer
	trigger.UnsafeAlertingServer

	Ctx        context.Context
	Logger     *zap.SugaredLogger
	inMemCache *lru.Cache

	opsNode     *ops.AlertingOpsNode
	storageNode *alertstorage.StorageNode

	mgmtClient  future.Future[managementv1.ManagementClient]
	adminClient future.Future[cortexadmin.CortexAdminClient]
}

type StorageAPIs struct {
	Conditions    storage.KeyValueStoreT[*alertingv1alpha.AlertCondition]
	AlertEndpoint storage.KeyValueStoreT[*alertingv1alpha.AlertEndpoint]
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewPluginLogger().Named("alerting")
	clusterDriver := future.New[drivers.ClusterDriver]()
	return &Plugin{
		Ctx:         ctx,
		Logger:      lg,
		inMemCache:  nil,
		mgmtClient:  future.New[managementv1.ManagementClient](),
		adminClient: future.New[cortexadmin.CortexAdminClient](),
		opsNode:     ops.NewAlertingOpsNode(clusterDriver),
		storageNode: alertstorage.NewStorageNode(
			alertstorage.WithLogger(lg),
		),
	}
}

var _ endpoint.AlertEndpointsServer = (*Plugin)(nil)
var _ condition.AlertConditionsServer = (*Plugin)(nil)
var _ log.AlertLogsServer = (*Plugin)(nil)
var _ trigger.AlertingServer = (*Plugin)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	// We omit the trigger server here since it should only be used internally
	scheme.Add(managementext.ManagementAPIExtensionPluginID,
		managementext.NewPlugin(
			util.PackService(
				&condition.AlertConditions_ServiceDesc,
				p,
			),
			util.PackService(
				&endpoint.AlertEndpoints_ServiceDesc,
				p,
			),
			util.PackService(
				&log.AlertLogs_ServiceDesc,
				p,
			),
			util.PackService(
				&alertops.AlertingAdmin_ServiceDesc,
				p.opsNode,
			),
		),
	)
	return scheme
}
