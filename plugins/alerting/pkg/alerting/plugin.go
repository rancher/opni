package alerting

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/rancher/opni/pkg/management"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/alerting/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alarms/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/endpoints/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/messaging"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/notifications/v1"
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

type ServerComponent interface {
	util.InitializerF
	SetConfig(config struct{})
	// Reports if the state of the component is running
	Status() struct{}
	// Server components that manage independent dependencies
	// should implement this method to sync them
	Sync(enabled bool) error
}

const AlertingLogCacheSize = 32

func (p *Plugin) Components() []ServerComponent {
	return []ServerComponent{
		p.NotificationServerComponent,
		p.EndpointServerComponent,
		p.AlarmServerComponent,
	}
}

type Plugin struct {
	system.UnimplementedSystemPluginClient

	Ctx    context.Context
	Logger *zap.SugaredLogger

	// components       serverComponents
	opsNode          *ops.AlertingOpsNode
	msgNode          *messaging.MessagingNode
	storageClientSet future.Future[storage.AlertingClientSet]

	// FIXME: deprecate me
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

	*notifications.NotificationServerComponent
	*endpoints.EndpointServerComponent
	*alarms.AlarmServerComponent
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
	p.NotificationServerComponent = notifications.NewNotificationServerComponent(
		p.Logger.With("component", "notifications"),
	)
	p.EndpointServerComponent = endpoints.NewEndpointServerComponent(
		p.Ctx,
		p.Logger.With("component", "endpoints"),
		p.NotificationServerComponent,
	)
	p.AlarmServerComponent = alarms.NewAlarmServerComponent(
		p.Ctx,
		p.Logger.With("component", "alarms"),
		p.NotificationServerComponent,
	)

	future.Wait1(p.storageClientSet, func(s storage.AlertingClientSet) {
		p.NotificationServerComponent.Initialize(notifications.NotificationServerConfiguration{
			ConditionStorage: s.Conditions(),
			OpsNode:          p.opsNode,
		})

		p.EndpointServerComponent.Initialize(endpoints.EndpointServerConfiguration{
			ConditionStorage: s.Conditions(),
			OpsNode:          p.opsNode,
			EndpointStorage:  s.Endpoints(),
			RouterStorage:    s.Routers(),
		})
	})

	future.Wait5(p.js, p.storageClientSet, p.mgmtClient, p.adminClient, p.cortexOpsClient,
		func(
			js nats.JetStreamContext,
			s storage.AlertingClientSet,
			mgmtClient managementv1.ManagementClient,
			adminClient cortexadmin.CortexAdminClient,
			cortexOpsClient cortexops.CortexOpsClient) {
			p.AlarmServerComponent.Initialize(alarms.AlarmServerConfiguration{
				ConditionStorage: s.Conditions(),
				IncidentStorage:  s.Incidents(),
				StateStorage:     s.States(),
				RouterStorage:    s.Routers(),
				OpsNode:          p.opsNode,
				MgmtClient:       mgmtClient,
				AdminClient:      adminClient,
				CortexOpsClient:  cortexOpsClient,
				Js:               js,
			})
		})

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

	// TODO : iterate over server components and add them to the scheme

	scheme.Add(managementext.ManagementAPIExtensionPluginID,
		managementext.NewPlugin(
			util.PackService(
				&alertingv1.AlertConditions_ServiceDesc,
				p.AlarmServerComponent,
			),
			util.PackService(
				&alertingv1.AlertEndpoints_ServiceDesc,
				p.EndpointServerComponent,
			),
			util.PackService(
				&alertingv1.AlertNotifications_ServiceDesc,
				p.NotificationServerComponent,
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
