package alerting

import (
	"context"

	"github.com/rancher/opni/pkg/agent"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/management"
	"github.com/rancher/opni/pkg/metrics/collector"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	metricsExporter "github.com/rancher/opni/plugins/alerting/pkg/gateway/metrics"
	"github.com/rancher/opni/plugins/alerting/pkg/gateway/proxy"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/server"
	"github.com/rancher/opni/pkg/alerting/storage/spec"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
	"github.com/rancher/opni/pkg/plugins/apis/metrics"
	"github.com/rancher/opni/plugins/alerting/pkg/gateway/alarms/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/gateway/endpoints/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/gateway/notifications/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/node_backend"
	"go.uber.org/zap"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
	"github.com/rancher/opni/plugins/alerting/pkg/gateway/drivers"
)

func (p *Plugin) Components() []server.ServerComponent {
	return []server.ServerComponent{
		p.NotificationServerComponent,
		p.EndpointServerComponent,
		p.AlarmServerComponent,
		p.httpProxy,
	}
}

type Plugin struct {
	alertops.UnsafeAlertingAdminServer
	alertops.ConfigReconcilerServer
	system.UnimplementedSystemPluginClient

	ctx    context.Context
	logger *zap.SugaredLogger

	storageClientSet future.Future[spec.AlertingClientSet]

	client.AlertingClient
	clusterNotifier chan client.AlertingClient
	clusterDriver   future.Future[drivers.ClusterDriver]
	syncController  SyncController

	mgmtClient          future.Future[managementv1.ManagementClient]
	storageBackend      future.Future[storage.Backend]
	capabilitySpecStore future.Future[node_backend.CapabilitySpecKV]
	delegate            future.Future[streamext.StreamDelegate[agent.ClientSet]]
	adminClient         future.Future[cortexadmin.CortexAdminClient]
	cortexOpsClient     future.Future[cortexops.CortexOpsClient]
	natsConn            future.Future[*nats.Conn]
	js                  future.Future[nats.JetStreamContext]
	globalWatchers      management.ConditionWatcher

	gatewayConfig future.Future[*v1beta1.GatewayConfig]

	collector.CollectorServer

	*notifications.NotificationServerComponent
	*endpoints.EndpointServerComponent
	*alarms.AlarmServerComponent

	node node_backend.AlertingNodeBackend

	httpProxy *proxy.ProxyServer
	hsServer  *healthStatusServer
}

// ManagementServices implements managementext.ManagementAPIExtension.
func (p *Plugin) ManagementServices(_ managementext.ServiceController) []util.ServicePackInterface {
	return []util.ServicePackInterface{
		util.PackService[alertingv1.AlertConditionsServer](
			&alertingv1.AlertConditions_ServiceDesc,
			p.AlarmServerComponent,
		),
		util.PackService[alertingv1.AlertEndpointsServer](
			&alertingv1.AlertEndpoints_ServiceDesc,
			p.EndpointServerComponent,
		),
		util.PackService[alertingv1.AlertNotificationsServer](
			&alertingv1.AlertNotifications_ServiceDesc,
			p.NotificationServerComponent,
		),
		util.PackService[alertops.AlertingAdminServer](
			&alertops.AlertingAdmin_ServiceDesc,
			p,
		),
		util.PackService[alertops.ConfigReconcilerServer](
			&alertops.ConfigReconciler_ServiceDesc,
			p,
		),
		util.PackService[node.AlertingNodeConfigurationServer](
			&node.AlertingNodeConfiguration_ServiceDesc,
			&p.node,
		),
		util.PackService[node.NodeAlertingCapabilityServer](
			&node.NodeAlertingCapability_ServiceDesc,
			&p.node,
		),
	}
}

var (
	_ alertingv1.AlertEndpointsServer     = (*Plugin)(nil)
	_ alertingv1.AlertConditionsServer    = (*Plugin)(nil)
	_ alertingv1.AlertNotificationsServer = (*Plugin)(nil)
)

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewPluginLogger().Named("alerting")
	storageClientSet := future.New[spec.AlertingClientSet]()
	metricReader := metricsdk.NewManualReader()
	metricsExporter.RegisterMeterProvider(metricsdk.NewMeterProvider(
		metricsdk.WithReader(metricReader),
	))
	collector := collector.NewCollectorServer(metricReader)
	alertingClient, err := client.NewClient()
	if err != nil {
		panic(err)
	}
	p := &Plugin{
		ctx:    ctx,
		logger: lg,

		storageClientSet: storageClientSet,

		clusterNotifier: make(chan client.AlertingClient),
		clusterDriver:   future.New[drivers.ClusterDriver](),

		mgmtClient:          future.New[managementv1.ManagementClient](),
		storageBackend:      future.New[storage.Backend](),
		capabilitySpecStore: future.New[node_backend.CapabilitySpecKV](),
		delegate:            future.New[streamext.StreamDelegate[agent.ClientSet]](),

		adminClient:     future.New[cortexadmin.CortexAdminClient](),
		cortexOpsClient: future.New[cortexops.CortexOpsClient](),
		natsConn:        future.New[*nats.Conn](),
		js:              future.New[nats.JetStreamContext](),

		gatewayConfig: future.New[*v1beta1.GatewayConfig](),

		CollectorServer: collector,

		AlertingClient: alertingClient,
	}

	p.syncController = NewSyncController(p.logger.With("component", "sync-controller"))
	p.hsServer = newHealthStatusServer(
		p.ready,
		p.healthy,
	)
	p.httpProxy = proxy.NewProxyServer(
		lg.With("component", "http-proxy"),
	)

	p.node = *node_backend.NewAlertingNodeBackend(
		p.logger.With("component", "node-backend"),
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

	future.Wait4(
		p.storageBackend,
		p.mgmtClient,
		p.capabilitySpecStore,
		p.delegate,
		func(
			storageBackend storage.Backend,
			mgmtClient managementv1.ManagementClient,
			specStore node_backend.CapabilitySpecKV,
			delegate streamext.StreamDelegate[agent.ClientSet],
		) {
			p.node.Initialize(specStore, mgmtClient, delegate, storageBackend)
		},
	)

	future.Wait1(p.storageClientSet, func(s spec.AlertingClientSet) {
		p.NotificationServerComponent.Initialize(notifications.NotificationServerConfiguration{
			ConditionStorage: s.Conditions(),
			EndpointStorage:  s.Endpoints(),
		})

		p.EndpointServerComponent.Initialize(endpoints.EndpointServerConfiguration{
			ConditionStorage: s.Conditions(),
			EndpointStorage:  s.Endpoints(),
			RouterStorage:    s.Routers(),
			HashRing:         s,
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
			s spec.AlertingClientSet,
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

var (
	_ alertingv1.AlertEndpointsServer     = (*Plugin)(nil)
	_ alertingv1.AlertConditionsServer    = (*Plugin)(nil)
	_ alertingv1.AlertNotificationsServer = (*Plugin)(nil)
)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(httpext.HTTPAPIExtensionPluginID, httpext.NewPlugin(p.httpProxy))
	scheme.Add(managementext.ManagementAPIExtensionPluginID, managementext.NewPlugin(p))
	scheme.Add(metrics.MetricsPluginID, metrics.NewPlugin(p))
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewPlugin(&p.node))
	scheme.Add(streamext.StreamAPIExtensionPluginID, streamext.NewGatewayPlugin(p))
	return scheme
}
