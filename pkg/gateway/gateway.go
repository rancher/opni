package gateway

import (
	"context"
	"crypto/tls"
	"net"

	"github.com/gofiber/fiber/v2"
	"github.com/hashicorp/go-plugin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/opni/pkg/agent"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/meta"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/pkg/webui"
	"github.com/soheilhy/cmux"
	"go.uber.org/zap"
	"golang.org/x/mod/module"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/emptypb"
)

type APIExtensionPlugin = plugins.TypedActivePlugin[apiextensions.GatewayAPIExtensionClient]
type CapabilityBackendPlugin = plugins.TypedActivePlugin[capability.BackendClient]
type SystemPlugin = plugins.TypedActivePlugin[system.SystemPluginServer]
type MetricsPlugin = plugins.TypedActivePlugin[prometheus.Collector]

type Gateway struct {
	GatewayOptions
	config       *config.GatewayConfig
	ctx          context.Context
	tlsConfig    *tls.Config
	logger       *zap.SugaredLogger
	apiServer    *GatewayAPIServer
	streamServer *StreamServer

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

	_, port, err := net.SplitHostPort(conf.Spec.ListenAddress)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to parse listen address")
	}

	capBackendStore := capabilities.NewBackendStore(capabilities.ServerInstallerTemplateSpec{
		Address: "https://" + conf.Spec.Hostname + ":" + port,
	}, lg)

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

	tlsConfig, err := loadTLSConfig(&conf.Spec)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to load TLS config")
	}

	apiServer := NewAPIServer(ctx, &conf.Spec, lg, options.apiServerOptions...)
	apiServer.ConfigureBootstrapRoutes(storageBackend, capBackendStore)

	g := &Gateway{
		GatewayOptions:  options,
		ctx:             ctx,
		tlsConfig:       tlsConfig,
		config:          conf,
		logger:          lg,
		storageBackend:  storageBackend,
		capBackendStore: capBackendStore,
		apiServer:       apiServer,
	}

	clusterAuth, err := cluster.New(ctx, storageBackend, "authorization")
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to create stream interceptor")
	}

	streamServer := NewStreamServer(&conf.Spec, lg, g,
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.StreamInterceptor(clusterAuth.StreamServerInterceptor()),
	)
	g.streamServer = streamServer

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

	listener, err := tls.Listen("tcp4",
		g.config.Spec.ListenAddress, g.tlsConfig)
	if err != nil {
		lg.Fatal(err)
	}

	cl := cmux.New(listener)
	http1 := cl.Match(cmux.TLS(tls.VersionTLS12, tls.VersionTLS13), cmux.HTTP1())
	http2 := cl.Match(cmux.TLS(tls.VersionTLS12, tls.VersionTLS13), cmux.HTTP2())

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

	go func() {
		if err := g.apiServer.Serve(http1); err != nil {
			lg.With(
				zap.Error(err),
			).Error("API server exited with error")
		}
	}()
	go func() {
		if err := g.streamServer.Serve(http2); err != nil {
			lg.With(
				zap.Error(err),
			).Error("stream server exited with error")
		}
	}()
	return cl.Serve()
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

func (g *Gateway) MustRegisterCollector(collector prometheus.Collector) {
	g.apiServer.metricsHandler.MustRegister(collector)
}

func (g *Gateway) HandleAgentConnection(ctx context.Context, clientset agent.ClientSet) {
	g.logger.Info("Agent connected")
	defer g.logger.Info("Agent disconnected")
	health, err := clientset.GetHealth(ctx, &emptypb.Empty{})
	if err != nil {
		g.logger.Error(err)
	} else {
		g.logger.Info("Agent health: " + health.String())
	}
	<-ctx.Done()
}

func loadTLSConfig(cfg *v1beta1.GatewayConfigSpec) (*tls.Config, error) {
	servingCertBundle, caPool, err := util.LoadServingCertBundle(cfg.Certs)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      caPool,
		Certificates: []tls.Certificate{*servingCertBundle},
	}, nil
}
