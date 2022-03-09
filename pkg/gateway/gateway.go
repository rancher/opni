package gateway

import (
	"context"
	"crypto/tls"

	"github.com/gofiber/fiber/v2"
	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni-monitoring/pkg/config"
	"github.com/rancher/opni-monitoring/pkg/config/meta"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/system"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/waitctx"
	"github.com/rancher/opni-monitoring/pkg/webui"
	"go.uber.org/zap"
	"golang.org/x/mod/module"
)

type APIExtensionPlugin = plugins.TypedActivePlugin[apiextensions.GatewayAPIExtensionClient]
type SystemPlugin = plugins.TypedActivePlugin[system.SystemPluginServer]

type Gateway struct {
	GatewayOptions
	config *config.GatewayConfig
	ctx    context.Context
	logger *zap.SugaredLogger

	storageBackend  storage.Backend
	capBackendStore *CapabilityBackendStore

	apiServer *GatewayAPIServer

	cortexTLSConfig *tls.Config // *** temporary code ***
}

type GatewayOptions struct {
	apiServerOptions []APIServerOption
	lifecycler       config.Lifecycler
	systemPlugins    []SystemPlugin
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

func WithSystemPlugins(plugins []SystemPlugin) GatewayOption {
	return func(o *GatewayOptions) {
		o.systemPlugins = plugins
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

	storageBackend, err := ConfigureStorageBackend(ctx, &conf.Spec.Storage)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Error("failed to configure storage backend")
	}

	apiServer := NewAPIServer(ctx, &conf.Spec, lg, options.apiServerOptions...)
	apiServer.ConfigureBootstrapRoutes(storageBackend)

	g := &Gateway{
		GatewayOptions:  options,
		ctx:             ctx,
		config:          conf,
		logger:          lg,
		storageBackend:  storageBackend,
		capBackendStore: NewCapabilityBackendStore(lg),
		apiServer:       apiServer,
	}

	// ************************************************
	// temporary code
	g.loadCortexCerts()
	g.setupCortexRoutes(storageBackend, storageBackend)
	// ************************************************

	waitctx.Go(ctx, func() {
		<-ctx.Done()
		lg.Info("shutting down plugins")
		plugin.CleanupClients()
	})

	return g
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
		go systemPlugin.Typed.ServeKeyValueStore(store)
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
