package alerting

import (
	"context"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/management"
	"github.com/rancher/opni/pkg/metrics/collector"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	metricsExporter "github.com/rancher/opni/plugins/alerting/pkg/alerting/metrics"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/storage"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
	"github.com/rancher/opni/pkg/plugins/apis/metrics"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alarms/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/endpoints/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/notifications/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/server"
	"go.uber.org/zap"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
)

func (p *Plugin) Components() []server.ServerComponent {
	return []server.ServerComponent{
		p.NotificationServerComponent,
		p.EndpointServerComponent,
		p.AlarmServerComponent,
	}
}

type Plugin struct {
	alertops.UnsafeAlertingAdminServer
	alertops.ConfigReconcilerServer
	system.UnimplementedSystemPluginClient

	ctx    context.Context
	logger *zap.SugaredLogger

	storageClientSet future.Future[storage.AlertingClientSet]

	client.AlertingClient
	clusterNotifier chan []client.AlertingPeer
	clusterDriver   future.Future[drivers.ClusterDriver]
	syncController  SyncController

	mgmtClient      future.Future[managementv1.ManagementClient]
	adminClient     future.Future[cortexadmin.CortexAdminClient]
	cortexOpsClient future.Future[cortexops.CortexOpsClient]
	natsConn        future.Future[*nats.Conn]
	js              future.Future[nats.JetStreamContext]
	globalWatchers  management.ConditionWatcher

	gatewayConfig future.Future[*v1beta1.GatewayConfig]

	collector.CollectorServer

	*notifications.NotificationServerComponent
	*endpoints.EndpointServerComponent
	*alarms.AlarmServerComponent

	httpProxy *HttpApiServer
}

var (
	_ alertingv1.AlertEndpointsServer     = (*Plugin)(nil)
	_ alertingv1.AlertConditionsServer    = (*Plugin)(nil)
	_ alertingv1.AlertNotificationsServer = (*Plugin)(nil)
)

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewPluginLogger().Named("alerting")
	storageClientSet := future.New[storage.AlertingClientSet]()
	metricReader := metricsdk.NewManualReader()
	metricsExporter.RegisterMeterProvider(metricsdk.NewMeterProvider(
		metricsdk.WithReader(metricReader),
	))
	collector := collector.NewCollectorServer(metricReader)
	p := &Plugin{
		ctx:    ctx,
		logger: lg,

		storageClientSet: storageClientSet,

		clusterNotifier: make(chan []client.AlertingPeer),
		clusterDriver:   future.New[drivers.ClusterDriver](),

		mgmtClient:      future.New[managementv1.ManagementClient](),
		adminClient:     future.New[cortexadmin.CortexAdminClient](),
		cortexOpsClient: future.New[cortexops.CortexOpsClient](),
		natsConn:        future.New[*nats.Conn](),
		js:              future.New[nats.JetStreamContext](),

		gatewayConfig: future.New[*v1beta1.GatewayConfig](),

		CollectorServer: collector,

		AlertingClient: client.NewClient(
			nil,
			"http://opni-alerting:9093",
			"http://opni-alerting:3000",
		),
	}

	p.syncController = NewSyncController()
	p.httpProxy = NewHttpApiServer(
		lg.With("component", "http-proxy"),
		p.AlertingClient,
		p.ready,
		p.healthy,
	)
	p.NotificationServerComponent = notifications.NewNotificationServerComponent(
		p.logger.With("component", "notifications"),
	)
	p.EndpointServerComponent = endpoints.NewEndpointServerComponent(
		p.ctx,
		p.logger.With("component", "endpoints"),
		p.NotificationServerComponent,
	)
	p.AlarmServerComponent = alarms.NewAlarmServerComponent(
		p.ctx,
		p.logger.With("component", "alarms"),
		p.NotificationServerComponent,
	)

	future.Wait1(p.storageClientSet, func(s storage.AlertingClientSet) {
		p.NotificationServerComponent.Initialize(notifications.NotificationServerConfiguration{
			ConditionStorage: s.Conditions(),
		})

		p.EndpointServerComponent.Initialize(endpoints.EndpointServerConfiguration{
			ConditionStorage: s.Conditions(),
			EndpointStorage:  s.Endpoints(),
			RouterStorage:    s.Routers(),
			ManualSync:       p.SendManualSyncRequest,
		})

		serverCfg := server.Config{
			Client: p.AlertingClient,
		}
		p.NotificationServerComponent.SetConfig(
			serverCfg,
		)

		p.EndpointServerComponent.SetConfig(
			serverCfg,
		)
	})

	future.Wait5(p.js, p.storageClientSet, p.mgmtClient, p.adminClient, p.cortexOpsClient,
		func(
			js nats.JetStreamContext,
			s storage.AlertingClientSet,
			mgmtClient managementv1.ManagementClient,
			adminClient cortexadmin.CortexAdminClient,
			cortexOpsClient cortexops.CortexOpsClient,
		) {
			p.AlarmServerComponent.Initialize(alarms.AlarmServerConfiguration{
				ConditionStorage: s.Conditions(),
				IncidentStorage:  s.Incidents(),
				StateStorage:     s.States(),
				RouterStorage:    s.Routers(),
				MgmtClient:       mgmtClient,
				AdminClient:      adminClient,
				CortexOpsClient:  cortexOpsClient,
				Js:               js,
			})

			serverCfg := server.Config{
				Client: p.AlertingClient,
			}

			p.AlarmServerComponent.SetConfig(
				serverCfg,
			)
		})
	return p
}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(httpext.HTTPAPIExtensionPluginID, httpext.NewPlugin(p.httpProxy))
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
				p,
			),
			util.PackService(
				&alertops.ConfigReconciler_ServiceDesc,
				p,
			),
		),
	)
	scheme.Add(metrics.MetricsPluginID, metrics.NewPlugin(p))
	return scheme
}
