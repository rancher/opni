package alerting

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/rancher/opni/pkg/management"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	alertopsv2 "github.com/rancher/opni/plugins/alerting/pkg/apis/alertops/v2"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/alerting/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/messaging"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/ops"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	"go.uber.org/zap"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
)

const AlertingLogCacheSize = 32

type Plugin struct {
	system.UnimplementedSystemPluginClient
	alertingv1.UnsafeAlertConditionsServer
	alertingv1.UnsafeAlertEndpointsServer
	alertingv1.UnsafeAlertNotificationsServer
	alertopsv2.UnsafeAlertingAdminV2Server

	Ctx    context.Context
	Logger *zap.SugaredLogger

	opsNode          *ops.AlertingOpsNode
	msgNode          *messaging.MessagingNode
	storageClientSet future.Future[storage.AlertingClientSet]

	clusterOptionMu sync.RWMutex
	clusterOptions  *shared.AlertingClusterOptions
	clusterNotifier chan shared.AlertingClusterNotification
	// signifies that the Alerting Cluster is running and we should
	// be reconciling external dependencies in order to evaluate
	// alerts
	evaluating *atomic.Bool

	mgmtClient      future.Future[managementv1.ManagementClient]
	adminClient     future.Future[cortexadmin.CortexAdminClient]
	cortexOpsClient future.Future[cortexops.CortexOpsClient]
	natsConn        future.Future[*nats.Conn]
	js              future.Future[nats.JetStreamContext]
	globalWatchers  management.ConditionWatcher
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewPluginLogger().Named("alerting")
	clusterDriver := future.New[drivers.ClusterDriver]()
	storageClientSet := future.New[storage.AlertingClientSet]()
	defaultEvaluationState := &atomic.Bool{}
	defaultEvaluationState.Store(false)
	p := &Plugin{
		Ctx:    ctx,
		Logger: lg,

		opsNode:          ops.NewAlertingOpsNode(ctx, clusterDriver, storageClientSet),
		msgNode:          messaging.NewMessagingNode(),
		storageClientSet: storageClientSet,

		clusterNotifier: make(chan shared.AlertingClusterNotification),
		evaluating:      defaultEvaluationState,

		mgmtClient:      future.New[managementv1.ManagementClient](),
		adminClient:     future.New[cortexadmin.CortexAdminClient](),
		cortexOpsClient: future.New[cortexops.CortexOpsClient](),
		natsConn:        future.New[*nats.Conn](),
		js:              future.New[nats.JetStreamContext](),
	}
	return p
}

var _ alertingv1.AlertEndpointsServer = (*Plugin)(nil)
var _ alertingv1.AlertConditionsServer = (*Plugin)(nil)
var _ alertingv1.AlertNotificationsServer = (*Plugin)(nil)

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
				&alertingv1.AlertNotifications_ServiceDesc,
				p,
			),
			util.PackService(
				&alertops.AlertingAdmin_ServiceDesc,
				p.opsNode,
			),
			util.PackService(
				&alertops.ConfigReconciler_ServiceDesc,
				p.opsNode,
			),
		),
	)
	return scheme
}
