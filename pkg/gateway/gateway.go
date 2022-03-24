package gateway

import (
	"context"
	"crypto/tls"

	"github.com/gofiber/fiber/v2"
	"github.com/hashicorp/go-plugin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/opni-monitoring/pkg/capabilities"
	"github.com/rancher/opni-monitoring/pkg/config"
	"github.com/rancher/opni-monitoring/pkg/config/meta"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/machinery"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/capability"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/system"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/util/waitctx"
	"github.com/rancher/opni-monitoring/pkg/webui"
	"go.uber.org/zap"
	"golang.org/x/mod/module"
	"google.golang.org/protobuf/types/known/emptypb"
)

type APIExtensionPlugin = plugins.TypedActivePlugin[apiextensions.GatewayAPIExtensionClient]
type CapabilityBackendPlugin = plugins.TypedActivePlugin[capability.BackendClient]
type SystemPlugin = plugins.TypedActivePlugin[system.SystemPluginServer]
type MetricsPlugin = plugins.TypedActivePlugin[prometheus.Collector]

type Gateway struct {
	GatewayOptions
	config    *config.GatewayConfig
	ctx       context.Context
	logger    *zap.SugaredLogger
	apiServer *GatewayAPIServer

	storageBackend  storage.Backend
	capBackendStore capabilities.BackendStore
}

type GatewayOptions struct {
	apiServerOptions  []APIServerOption
	lifecycler        config.Lifecycler
	systemPlugins     []plugins.ActivePlugin
	capBackendPlugins []CapabilityBackendPlugin
}

type GatewayOption func(*GatewayOptions)

func (o *GatewayOptions) Apply(opts ...GatewayOption) {
	for _, op := range opts {
		op(o)
	}
}

type FiberMiddleware = func(*fiber.Ctx) error

func WithLifecycler(lc config.Lifecycler) GatewayOption {
	return func(o *GatewayOptions) {
		o.lifecycler = lc
	}
}

func WithSystemPlugins(plugins []plugins.ActivePlugin) GatewayOption {
	return func(o *GatewayOptions) {
		o.systemPlugins = plugins
	}
}

func WithCapabilityBackendPlugins(plugins []CapabilityBackendPlugin) GatewayOption {
	return func(o *GatewayOptions) {
		o.capBackendPlugins = plugins
	}
}

func WithAPIServerOptions(opts ...APIServerOption) GatewayOption {
	return func(o *GatewayOptions) {
		o.apiServerOptions = opts
	}
}

func NewGateway(ctx context.Context, conf *config.GatewayConfig, opts ...GatewayOption) *Gateway {
	options := GatewayOptions{
		lifecycler: config.NewUnavailableLifecycler(meta.ObjectList{conf}),
	}
	options.Apply(opts...)

	lg := logger.New().Named("gateway")

	conf.Spec.SetDefaults()

	storageBackend, err := machinery.ConfigureStorageBackend(ctx, &conf.Spec.Storage)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Error("failed to configure storage backend")
	}

	capBackendStore := capabilities.NewBackendStore(lg)
	for _, p := range options.capBackendPlugins {
		info, err := p.Typed.Info(ctx, &emptypb.Empty{})
		if err != nil {
			lg.With(
				zap.String("plugin", p.Metadata.Module),
			).Error("failed to get capability info")
			continue
		}
		backend := capabilities.NewBackend(p.Typed)
		if err := capBackendStore.Add(info.CapabilityName, backend); err != nil {
			lg.With(
				zap.String("plugin", p.Metadata.Module),
				zap.Error(err),
			).Error("failed to add capability backend")
		}
	}

	apiServer := NewAPIServer(ctx, &conf.Spec, lg, options.apiServerOptions...)
	apiServer.ConfigureBootstrapRoutes(storageBackend, capBackendStore)

	g := &Gateway{
		GatewayOptions:  options,
		ctx:             ctx,
		config:          conf,
		logger:          lg,
		storageBackend:  storageBackend,
		capBackendStore: capBackendStore,
		apiServer:       apiServer,
	}

	waitctx.Go(ctx, func() {
		<-ctx.Done()
		lg.Info("shutting down plugins")
		plugin.CleanupClients()
	})

	return g
}

type keyValueStoreServer interface {
	ServeKeyValueStore(store storage.KeyValueStore)
}

func (g *Gateway) ListenAndServe() error {
	lg := g.logger
	waitctx.Go(g.ctx, func() {
		<-g.ctx.Done()
		lg.Info("shutting down gateway api")
		if err := g.apiServer.Shutdown(); err != nil {
			lg.With(
				zap.Error(err),
			).Error("error shutting down gateway api")
		}
	})

	webuiSrv, err := webui.NewWebUIServer(g.config)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Warn("not starting web ui server")
	} else {
		go func() {
			if err := webuiSrv.ListenAndServe(); err != nil {
				lg.With(
					zap.Error(err),
				).Warn("ui server stopped")
			}
		}()
		waitctx.Go(g.ctx, func() {
			<-g.ctx.Done()
			lg.Info("shutting down ui server")
			if err := webuiSrv.Shutdown(context.Background()); err != nil {
				lg.With(
					zap.Error(err),
				).Error("error shutting down ui server")
			}
		})
	}

	g.logger.Infof("serving management api for %d system plugins", len(g.systemPlugins))
	for _, systemPlugin := range g.systemPlugins {
		ns := systemPlugin.Metadata.Module
		if err := module.CheckPath(ns); err != nil {
			g.logger.With(
				zap.String("namespace", ns),
				zap.Error(err),
			).Warn("system plugin module name is invalid")
			continue
		}
		store, err := g.storageBackend.KeyValueStore(ns)
		if err != nil {
			return err
		}
		go systemPlugin.Raw.(keyValueStoreServer).ServeKeyValueStore(store)
	}

	return g.apiServer.ListenAndServe()
}

// Implements management.CoreDataSource
func (g *Gateway) StorageBackend() storage.Backend {
	return g.storageBackend
}

// Implements management.CoreDataSource
func (g *Gateway) TLSConfig() *tls.Config {
	return g.apiServer.tlsConfig
}

// Implements management.CapabilitiesDataSource
func (g *Gateway) CapabilitiesStore() capabilities.BackendStore {
	return g.capBackendStore
}
