package gateway

import (
	"context"
	"sync"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/config/v1beta1"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"
	"github.com/rancher/opni/plugins/metrics/pkg/types"
	"go.uber.org/zap"
	"golang.org/x/tools/pkg/memoize"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type field[T any] struct {
	cf    func() chan T
	tf    func() T
	cInit sync.Once
	fInit sync.Once
}

// Returns a 1-buffered channel of type T. Writing to this channel will unblock
// any calls to F(), and all future calls to F() will return the same value.
func (f *field[T]) C() chan<- T {
	return f.c()
}

func (f *field[T]) c() chan T {
	f.cInit.Do(func() {
		f.cf = sync.OnceValue(func() chan T {
			return make(chan T, 1)
		})
	})
	return f.cf()
}

// Returns a value of type T written to C(), or blocks until it is available.
// All future calls to F() will return the same value. Thread-safe.
func (f *field[T]) F() T {
	f.fInit.Do(func() {
		f.tf = sync.OnceValue(func() T {
			return <-f.c()
		})
	})
	return f.tf()
}

type pluginContextData struct {
	managementClient    field[managementv1.ManagementClient]
	keyValueStoreClient field[system.KeyValueStoreClient]
	streamClient        field[grpc.ClientConnInterface]
	clusterDriver       field[drivers.ClusterDriver]
	storageBackend      field[storage.Backend]
	gatewayConfig       field[*v1beta1.GatewayConfig]
	delegate            field[streamext.StreamDelegate[types.MetricsAgentClientSet]]
	authMiddlewares     field[map[string]auth.Middleware]
	serviceCtrl         field[managementext.ServiceController]
	extensionClient     field[system.ExtensionClientInterface]
}

type pluginContext struct {
	context.Context
	logger     *zap.SugaredLogger
	store      *memoize.Store
	releasesMu sync.Mutex
	releases   []func()
	d          pluginContextData
}

func newPluginContext(ctx context.Context, logger *zap.SugaredLogger) (types.PluginContext, *pluginContextData) {
	pctx := pluginContext{
		Context: ctx,
		logger:  logger,
		store:   memoize.NewStore(memoize.ImmediatelyEvict),
	}
	context.AfterFunc(ctx, pctx.releaseAll)
	return &pctx, &pctx.d
}

func (c *pluginContext) ManagementClient() managementv1.ManagementClient {
	return c.d.managementClient.F()
}

func (c *pluginContext) KeyValueStoreClient() system.KeyValueStoreClient {
	return c.d.keyValueStoreClient.F()
}

func (c *pluginContext) StreamClient() grpc.ClientConnInterface {
	return c.d.streamClient.F()
}

func (c *pluginContext) ClusterDriver() drivers.ClusterDriver {
	return c.d.clusterDriver.F()
}

func (c *pluginContext) StorageBackend() storage.Backend {
	return c.d.storageBackend.F()
}

func (c *pluginContext) GatewayConfig() *v1beta1.GatewayConfig {
	return c.d.gatewayConfig.F()
}

func (c *pluginContext) Delegate() streamext.StreamDelegate[types.MetricsAgentClientSet] {
	return c.d.delegate.F()
}

func (c *pluginContext) AuthMiddlewares() map[string]auth.Middleware {
	return c.d.authMiddlewares.F()
}

func (c *pluginContext) ExtensionClient() system.ExtensionClientInterface {
	return c.d.extensionClient.F()
}

func (c *pluginContext) SetServingStatus(serviceName string, status grpc_health_v1.HealthCheckResponse_ServingStatus) {
	c.d.serviceCtrl.F().SetServingStatus(serviceName, status)
}

func (p *pluginContext) Logger() *zap.SugaredLogger {
	return p.logger
}

func (p *pluginContext) Memoize(key any, function memoize.Function) *memoize.Promise {
	promise, release := p.store.Promise(key, function)
	p.releasesMu.Lock()
	p.releases = append(p.releases, release)
	p.releasesMu.Unlock()
	return promise
}

func (p *pluginContext) releaseAll() {
	p.releasesMu.Lock()
	defer p.releasesMu.Unlock()
	for _, release := range p.releases {
		release()
	}
}

var (
	_ types.ManagementServiceContext = (*pluginContext)(nil)
	_ types.StreamServiceContext     = (*pluginContext)(nil)
)
