package gateway

import (
	"context"
	"crypto/tls"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/metrics/collector"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/metrics"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/backend"
	"github.com/rancher/opni/plugins/metrics/pkg/cortex"
	"github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/zap"
)

type Plugin struct {
	// capabilityv1.UnsafeBackendServer
	system.UnimplementedSystemPluginClient
	collector.CollectorServer

	ctx    context.Context
	logger *zap.SugaredLogger

	cortexAdmin       cortex.CortexAdminServer
	cortexHttp        cortex.HttpApiServer
	cortexRemoteWrite cortex.RemoteWriteForwarder
	metrics           backend.MetricsBackend
	rbac              backend.RBACBackend
	uninstallRunner   cortex.UninstallTaskRunner

	config              future.Future[*v1beta1.GatewayConfig]
	authMw              future.Future[map[string]auth.Middleware]
	mgmtClient          future.Future[managementv1.ManagementClient]
	storageBackend      future.Future[storage.Backend]
	cortexTlsConfig     future.Future[*tls.Config]
	cortexClientSet     future.Future[cortex.ClientSet]
	uninstallController future.Future[*task.Controller]
	clusterDriver       future.Future[drivers.ClusterDriver]
	delegate            future.Future[streamext.StreamDelegate[backend.MetricsAgentClientSet]]
	backendKvClients    future.Future[*backend.KVClients]
}

func NewPlugin(ctx context.Context) *Plugin {
	cortexReader := metric.NewManualReader(
		metric.WithAggregationSelector(cortex.CortexAggregationSelector),
	)
	mp := metric.NewMeterProvider(
		metric.WithReader(cortexReader),
	)
	cortex.RegisterMeterProvider(mp)

	collector := collector.NewCollectorServer(cortexReader)
	p := &Plugin{
		CollectorServer: collector,
		ctx:             ctx,
		logger:          logger.NewPluginLogger().Named("metrics"),

		config:              future.New[*v1beta1.GatewayConfig](),
		authMw:              future.New[map[string]auth.Middleware](),
		mgmtClient:          future.New[managementv1.ManagementClient](),
		storageBackend:      future.New[storage.Backend](),
		cortexTlsConfig:     future.New[*tls.Config](),
		cortexClientSet:     future.New[cortex.ClientSet](),
		uninstallController: future.New[*task.Controller](),
		clusterDriver:       future.New[drivers.ClusterDriver](),
		delegate:            future.New[streamext.StreamDelegate[backend.MetricsAgentClientSet]](),
		backendKvClients:    future.New[*backend.KVClients](),
	}
	p.metrics.OpsBackend = &backend.OpsServiceBackend{MetricsBackend: &p.metrics}
	p.metrics.NodeBackend = &backend.NodeServiceBackend{MetricsBackend: &p.metrics}

	future.Wait2(p.cortexClientSet, p.config,
		func(cortexClientSet cortex.ClientSet, config *v1beta1.GatewayConfig) {
			p.cortexAdmin.Initialize(cortex.CortexAdminServerConfig{
				CortexClientSet: cortexClientSet,
				Config:          &config.Spec,
				Logger:          p.logger.Named("cortex-admin"),
			})
		})

	future.Wait2(p.cortexClientSet, p.config,
		func(cortexClientSet cortex.ClientSet, config *v1beta1.GatewayConfig) {
			p.cortexRemoteWrite.Initialize(cortex.RemoteWriteForwarderConfig{
				CortexClientSet: cortexClientSet,
				Config:          &config.Spec,
				Logger:          p.logger.Named("cortex-rw"),
			})
		})

	future.Wait3(p.cortexClientSet, p.config, p.storageBackend,
		func(cortexClientSet cortex.ClientSet, config *v1beta1.GatewayConfig, storageBackend storage.Backend) {
			p.uninstallRunner.Initialize(cortex.UninstallTaskRunnerConfig{
				CortexClientSet: cortexClientSet,
				Config:          &config.Spec,
				StorageBackend:  storageBackend,
			})
		})
	future.Wait2(p.config, p.backendKvClients, func(
		config *v1beta1.GatewayConfig,
		backendKvClients *backend.KVClients,
	) {
		driverName := config.Spec.Cortex.Management.ClusterDriver
		if driverName == "" {
			p.logger.Warn("no cluster driver configured")
		}
		builder, ok := drivers.ClusterDrivers.Get(driverName)
		if !ok {
			p.logger.With(
				"driver", driverName,
			).Error("unknown cluster driver, using fallback noop driver")
			builder, ok = drivers.ClusterDrivers.Get("noop")
			if !ok {
				panic("bug: noop cluster driver not found")
			}
		}
		driver, err := builder(p.ctx,
			driverutil.NewOption("defaultConfigStore", backendKvClients.DefaultClusterConfigurationSpec),
		)
		if err != nil {
			p.logger.With(
				"driver", driverName,
				zap.Error(err),
			).Panic("failed to initialize cluster driver")
			return
		}
		p.logger.With(
			"driver", driverName,
		).Info("initialized cluster driver")
		p.clusterDriver.Set(driver)
	})
	future.Wait6(p.storageBackend, p.mgmtClient, p.uninstallController, p.clusterDriver, p.delegate, p.backendKvClients,
		func(
			storageBackend storage.Backend,
			mgmtClient managementv1.ManagementClient,
			uninstallController *task.Controller,
			clusterDriver drivers.ClusterDriver,
			delegate streamext.StreamDelegate[backend.MetricsAgentClientSet],
			backendKvClients *backend.KVClients,
		) {
			p.metrics.Initialize(backend.MetricsBackendConfig{
				Logger:              p.logger.Named("metrics-backend"),
				StorageBackend:      storageBackend,
				MgmtClient:          mgmtClient,
				UninstallController: uninstallController,
				ClusterDriver:       clusterDriver,
				Delegate:            delegate,
				KV:                  backendKvClients,
			})
		})

	future.Wait7(p.mgmtClient, p.cortexClientSet, p.config, p.cortexTlsConfig, p.storageBackend, p.authMw, p.backendKvClients,
		func(
			mgmtApi managementv1.ManagementClient,
			cortexClientSet cortex.ClientSet,
			config *v1beta1.GatewayConfig,
			tlsConfig *tls.Config,
			storageBackend storage.Backend,
			authMiddlewares map[string]auth.Middleware,
			kv *backend.KVClients,
		) {
			p.rbac.Initialize(backend.RBACBackendConfig{
				Logger:         p.logger.Named("rbac"),
				RoleStore:      kv.RolesStore,
				StorageBackend: storageBackend,
			})
			p.cortexHttp.Initialize(cortex.HttpApiServerConfig{
				PluginContext:    p.ctx,
				ManagementClient: mgmtApi,
				CortexClientSet:  cortexClientSet,
				Config:           &config.Spec,
				CortexTLSConfig:  tlsConfig,
				Logger:           p.logger.Named("cortex-http"),
				StorageBackend:   storageBackend,
				AuthMiddlewares:  authMiddlewares,
				RBAC:             &p.rbac,
			})
		})

	return p
}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeGateway))
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(httpext.HTTPAPIExtensionPluginID, httpext.NewPlugin(&p.cortexHttp))
	streamMetricReader := metric.NewManualReader()
	p.CollectorServer.AppendReader(streamMetricReader)
	scheme.Add(streamext.StreamAPIExtensionPluginID, streamext.NewGatewayPlugin(p,
		streamext.WithMetrics(streamext.GatewayStreamMetricsConfig{
			Reader:          streamMetricReader,
			LabelsForStream: p.labelsForStreamMetrics,
		})),
	)
	scheme.Add(managementext.ManagementAPIExtensionPluginID, managementext.NewPlugin(
		util.PackService(&cortexadmin.CortexAdmin_ServiceDesc, &p.cortexAdmin),
		util.PackService(&cortexops.CortexOps_ServiceDesc, p.metrics.OpsBackend),
		util.PackService(&remoteread.RemoteReadGateway_ServiceDesc, &p.metrics),
		util.PackService(&node.NodeConfiguration_ServiceDesc, p.metrics.NodeBackend),
	))
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewPlugin(&p.metrics))
	scheme.Add(capability.CapabilityRBACPluginID, capability.NewRBACPlugin(&p.rbac))
	scheme.Add(metrics.MetricsPluginID, metrics.NewPlugin(p))
	return scheme
}
