package alerting

import (
	"context"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/aiops/pkg/apis/modeltraining"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/messaging"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/ops"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	"go.uber.org/zap"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alertstorage"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/server/log"
)

const AlertingLogCacheSize = 32

type Plugin struct {
	system.UnimplementedSystemPluginClient
	alertingv1.UnsafeAlertConditionsServer
	alertingv1.UnsafeAlertEndpointsServer
	log.UnsafeAlertLogsServer
	alertingv1.UnsafeAlertingServer

	Ctx    context.Context
	Logger *zap.SugaredLogger

	opsNode     *ops.AlertingOpsNode
	msgNode     *messaging.MessagingNode
	storageNode *alertstorage.StorageNode

	mgmtClient          future.Future[managementv1.ManagementClient]
	adminClient         future.Future[cortexadmin.CortexAdminClient]
	modeltrainingClient future.Future[modeltraining.ModelTrainingClient]
	cortexOpsClient     future.Future[cortexops.CortexOpsClient]
	natsConn            future.Future[*nats.Conn]
	js                  future.Future[nats.JetStreamContext]
	globalWatchers      InternalConditionWatcher
}

type StorageAPIs struct {
	Conditions    storage.KeyValueStoreT[*alertingv1.AlertCondition]
	AlertEndpoint storage.KeyValueStoreT[*alertingv1.AlertEndpoint]
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewPluginLogger().Named("alerting")
	clusterDriver := future.New[drivers.ClusterDriver]()
	p := &Plugin{
		Ctx:    ctx,
		Logger: lg,

		opsNode: ops.NewAlertingOpsNode(clusterDriver),
		msgNode: messaging.NewMessagingNode(),
		storageNode: alertstorage.NewStorageNode(
			alertstorage.WithLogger(lg),
		),

		mgmtClient:          future.New[managementv1.ManagementClient](),
		adminClient:         future.New[cortexadmin.CortexAdminClient](),
		cortexOpsClient:     future.New[cortexops.CortexOpsClient](),
		modeltrainingClient: future.New[modeltraining.ModelTrainingClient](),
		natsConn:            future.New[*nats.Conn](),
		js:                  future.New[nats.JetStreamContext](),
	}
	return p
}

var _ alertingv1.AlertEndpointsServer = (*Plugin)(nil)
var _ alertingv1.AlertConditionsServer = (*Plugin)(nil)
var _ log.AlertLogsServer = (*Plugin)(nil)
var _ alertingv1.AlertingServer = (*Plugin)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	// We omit the trigger server here since it should only be used internally
	scheme.Add(managementext.ManagementAPIExtensionPluginID,
		managementext.NewPlugin(
			util.PackService(
				&alertingv1.AlertConditions_ServiceDesc,
				p,
			),
			util.PackService(
				&alertingv1.AlertEndpoints_ServiceDesc,
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
