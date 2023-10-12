package gateway

import (
	"context"
	"fmt"
	"sync"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/metrics/collector"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/metrics"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/cortex"
	"github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"
	"github.com/rancher/opni/plugins/metrics/pkg/types"
	"github.com/samber/lo"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/zap"
	"golang.org/x/tools/pkg/memoize"
	"google.golang.org/grpc"
)

type Plugin struct {
	// capabilityv1.UnsafeBackendServer
	system.UnimplementedSystemPluginClient
	collector.CollectorServer
	ctx context.Context

	logger *zap.SugaredLogger

	mgmtClientC          chan managementv1.ManagementClient
	keyValueStoreClientC chan system.KeyValueStoreClient
	streamClientC        chan grpc.ClientConnInterface
	clusterDriverC       chan drivers.ClusterDriver
	storageBackendC      chan storage.Backend
	delegateC            chan streamext.StreamDelegate[types.MetricsAgentClientSet]
	gatewayConfigC       chan *v1beta1.GatewayConfig
	authMiddlewaresC     chan map[string]auth.Middleware

	mgmtServiceRegistrarC chan chan func(util.ServicePackInterface) managementext.SingleServiceController
	streamServices        []util.ServicePackInterface
}

type pluginContext struct {
	context.Context
	logger *zap.SugaredLogger
	store  *memoize.Store

	managementClient    managementv1.ManagementClient
	keyValueStoreClient system.KeyValueStoreClient
	streamClient        grpc.ClientConnInterface
	clusterDriver       drivers.ClusterDriver
	storageBackend      storage.Backend
	gatewayConfig       *v1beta1.GatewayConfig
	delegate            streamext.StreamDelegate[types.MetricsAgentClientSet]
	authMiddlewares     map[string]auth.Middleware

	mgmtServiceRegistrar func(util.ServicePackInterface) managementext.SingleServiceController

	releasesMu sync.Mutex
	releases   []func()
}

func (pc *pluginContext) ManagementClient() managementv1.ManagementClient {
	return pc.managementClient
}

func (pc *pluginContext) KeyValueStoreClient() system.KeyValueStoreClient {
	return pc.keyValueStoreClient
}

func (pc *pluginContext) StreamClient() grpc.ClientConnInterface {
	return pc.streamClient
}

func (pc *pluginContext) ClusterDriver() drivers.ClusterDriver {
	return pc.clusterDriver
}

func (pc *pluginContext) StorageBackend() storage.Backend {
	return pc.storageBackend
}

func (pc *pluginContext) GatewayConfig() *v1beta1.GatewayConfig {
	return pc.gatewayConfig
}

func (pc *pluginContext) Delegate() streamext.StreamDelegate[types.MetricsAgentClientSet] {
	return pc.delegate
}

func (pc *pluginContext) AuthMiddlewares() map[string]auth.Middleware {
	return pc.authMiddlewares
}

func (pc *pluginContext) RegisterService(si util.ServicePackInterface) managementext.SingleServiceController {
	return pc.mgmtServiceRegistrar(si)
}

func (p *pluginContext) Logger() *zap.SugaredLogger {
	return p.logger
}

func (p *pluginContext) Memoize(key any, function func() (any, error)) (any, error) {
	promise, release := p.store.Promise(key, func(_ context.Context, _ any) any {
		return lo.T2(function())
	})
	p.releasesMu.Lock()
	p.releases = append(p.releases, release)
	p.releasesMu.Unlock()
	v, err := promise.Get(p, nil)
	if err != nil {
		return nil, err
	}
	return v.(lo.Tuple2[any, error]).Unpack()
}

func (p *pluginContext) releaseAll() {
	p.releasesMu.Lock()
	defer p.releasesMu.Unlock()
	for _, release := range p.releases {
		release()
	}
}

func NewPlugin(ctx context.Context, scheme meta.Scheme) *Plugin {
	cortexReader := sdkmetric.NewManualReader(
		sdkmetric.WithAggregationSelector(cortex.CortexAggregationSelector),
	)
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(cortexReader),
	)
	cortex.RegisterMeterProvider(mp)

	collector := collector.NewCollectorServer(cortexReader)
	p := &Plugin{
		ctx:             ctx,
		CollectorServer: collector,
		logger:          logger.NewPluginLogger().Named("metrics"),

		mgmtClientC:           make(chan managementv1.ManagementClient),
		keyValueStoreClientC:  make(chan system.KeyValueStoreClient),
		streamClientC:         make(chan grpc.ClientConnInterface),
		clusterDriverC:        make(chan drivers.ClusterDriver),
		storageBackendC:       make(chan storage.Backend),
		gatewayConfigC:        make(chan *v1beta1.GatewayConfig),
		delegateC:             make(chan streamext.StreamDelegate[types.MetricsAgentClientSet]),
		mgmtServiceRegistrarC: make(chan chan func(util.ServicePackInterface) managementext.SingleServiceController),
	}

	activations := []func(*pluginContext){}

	types.Services.Range(func(name string, builder driverutil.Builder[types.Service]) {
		lg := p.logger.With("service", name)
		svc, err := builder(ctx)
		if err != nil {
			lg.With(zap.Error(err)).Error("failed to initialize service")
			return
		}
		switch svc := svc.(type) {
		case types.PluginService:
			lg.Info("added service to scheme")
			svc.AddToScheme(scheme)
		case types.StreamService:
			p.streamServices = append(p.streamServices, svc.StreamServices()...)
		}
		lg.Info("initialized service")
		activations = append(activations, func(pctx *pluginContext) {
			if err := svc.Activate(pctx); err != nil {
				lg.With(zap.Error(err)).Error("failed to activate service")
			}
		})
	})

	go func() {
		pctx := &pluginContext{
			Context:             ctx,
			logger:              p.logger,
			store:               memoize.NewStore(memoize.ImmediatelyEvict),
			managementClient:    <-p.mgmtClientC,
			keyValueStoreClient: <-p.keyValueStoreClientC,
			streamClient:        <-p.streamClientC,
			storageBackend:      <-p.storageBackendC,
			delegate:            <-p.delegateC,
			gatewayConfig:       <-p.gatewayConfigC,
			authMiddlewares:     <-p.authMiddlewaresC,
		}
		context.AfterFunc(ctx, pctx.releaseAll)

		mgmtRegistrarC := make(chan func(util.ServicePackInterface) managementext.SingleServiceController)
		p.mgmtServiceRegistrarC <- mgmtRegistrarC
		pctx.mgmtServiceRegistrar = <-mgmtRegistrarC

		var err error
		pctx.clusterDriver, err = initClusterDriver(pctx)
		if err != nil {
			p.logger.With(zap.Error(err)).Error("failed to initialize cluster driver")
			return
		}

		for _, activate := range activations {
			activate(pctx)
		}

		close(mgmtRegistrarC)
	}()

	return p
}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme(meta.WithMode(meta.ModeGateway))
	p := NewPlugin(ctx, scheme)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	streamMetricReader := sdkmetric.NewManualReader()
	p.CollectorServer.AppendReader(streamMetricReader)
	scheme.Add(streamext.StreamAPIExtensionPluginID, streamext.NewGatewayPlugin(p,
		streamext.WithMetrics(streamext.GatewayStreamMetricsConfig{
			Reader:          streamMetricReader,
			LabelsForStream: p.labelsForStreamMetrics,
		})),
	)
	scheme.Add(managementext.ManagementAPIExtensionPluginID, managementext.NewPlugin(p))
	scheme.Add(metrics.MetricsPluginID, metrics.NewPlugin(p))
	return scheme
}

func initClusterDriver(ctx types.PluginContext) (drivers.ClusterDriver, error) {
	driverName := ctx.GatewayConfig().Spec.Cortex.Management.ClusterDriver
	if driverName == "" {
		return nil, fmt.Errorf("cluster driver not configured")
	}
	builder, ok := drivers.ClusterDrivers.Get(driverName)
	if !ok {
		return nil, fmt.Errorf("unknown cluster driver %q", driverName)
	}
	driver, err := builder(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cluster driver %q: %w", driverName, err)
	}
	ctx.Logger().With(
		"driver", driverName,
	).Info("initialized cluster driver")
	return driver, nil
}
