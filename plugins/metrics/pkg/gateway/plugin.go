package gateway

import (
	"context"
	"crypto/tls"

	gsatomic "github.com/kralicky/gpkg/sync/atomic"

	"go.uber.org/zap"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
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
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	"github.com/rancher/opni/plugins/metrics/pkg/backend"
	"github.com/rancher/opni/plugins/metrics/pkg/cortex"
	"github.com/rancher/opni/plugins/metrics/pkg/drivers"
)

type Plugin struct {
	cortexops.UnsafeCortexOpsServer
	// capabilityv1.UnsafeBackendServer
	system.UnimplementedSystemPluginClient
	collector.CollectorServer

	ctx    context.Context
	logger *zap.SugaredLogger

	cortexAdmin       cortex.CortexAdminServer
	cortexHttp        cortex.HttpApiServer
	cortexRemoteWrite cortex.RemoteWriteForwarder
	metrics           backend.MetricsBackend
	uninstallRunner   cortex.UninstallTaskRunner

	config              future.Future[*v1beta1.GatewayConfig]
	authMw              future.Future[map[string]auth.Middleware]
	mgmtClient          future.Future[managementv1.ManagementClient]
	nodeManagerClient   future.Future[capabilityv1.NodeManagerClient]
	storageBackend      future.Future[storage.Backend]
	cortexTlsConfig     future.Future[*tls.Config]
	cortexClientSet     future.Future[cortex.ClientSet]
	uninstallController future.Future[*task.Controller]
	clusterDriver       gsatomic.Value[drivers.ClusterDriver]
}

func NewPlugin(ctx context.Context) *Plugin {
	p := &Plugin{
		CollectorServer: collectorServer,
		ctx:             ctx,
		logger:          logger.NewPluginLogger().Named("metrics"),

		config:              future.New[*v1beta1.GatewayConfig](),
		authMw:              future.New[map[string]auth.Middleware](),
		mgmtClient:          future.New[managementv1.ManagementClient](),
		nodeManagerClient:   future.New[capabilityv1.NodeManagerClient](),
		storageBackend:      future.New[storage.Backend](),
		cortexTlsConfig:     future.New[*tls.Config](),
		cortexClientSet:     future.New[cortex.ClientSet](),
		uninstallController: future.New[*task.Controller](),
	}

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

	future.Wait4(p.storageBackend, p.mgmtClient, p.nodeManagerClient, p.uninstallController,
		func(
			storageBackend storage.Backend,
			mgmtClient managementv1.ManagementClient,
			nodeManagerClient capabilityv1.NodeManagerClient,
			uninstallController *task.Controller,
		) {
			p.metrics.Initialize(backend.MetricsBackendConfig{
				Logger:              p.logger.Named("metrics-backend"),
				StorageBackend:      storageBackend,
				MgmtClient:          mgmtClient,
				NodeManagerClient:   nodeManagerClient,
				UninstallController: uninstallController,
			})
		})

	future.Wait6(p.mgmtClient, p.cortexClientSet, p.config, p.cortexTlsConfig, p.storageBackend, p.authMw,
		func(
			mgmtApi managementv1.ManagementClient,
			cortexClientSet cortex.ClientSet,
			config *v1beta1.GatewayConfig,
			tlsConfig *tls.Config,
			storageBackend storage.Backend,
			authMiddlewares map[string]auth.Middleware,
		) {
			p.cortexHttp.Initialize(cortex.HttpApiServerConfig{
				PluginContext:    p.ctx,
				ManagementClient: mgmtApi,
				CortexClientSet:  cortexClientSet,
				Config:           &config.Spec,
				CortexTLSConfig:  tlsConfig,
				Logger:           p.logger.Named("cortex-http"),
				StorageBackend:   storageBackend,
				AuthMiddlewares:  authMiddlewares,
			})
		})
	return p
}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeGateway))
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(httpext.HTTPAPIExtensionPluginID, httpext.NewPlugin(&p.cortexHttp))
	scheme.Add(streamext.StreamAPIExtensionPluginID, streamext.NewPlugin(p))
	scheme.Add(managementext.ManagementAPIExtensionPluginID, managementext.NewPlugin(
		util.PackService(&cortexadmin.CortexAdmin_ServiceDesc, &p.cortexAdmin),
		util.PackService(&cortexops.CortexOps_ServiceDesc, p),
	))
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewPlugin(&p.metrics))
	scheme.Add(metrics.MetricsPluginID, metrics.NewPlugin(p))
	return scheme
}
