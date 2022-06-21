package gateway

import (
	"context"
	"crypto"
	"crypto/tls"
	"net"

	"github.com/hashicorp/go-plugin"
	"github.com/prometheus/client_golang/prometheus"
	bootstrapv1 "github.com/rancher/opni/pkg/apis/bootstrap/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/config"
	cfgmeta "github.com/rancher/opni/pkg/config/meta"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/plugins/types"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/pkg/webui"
	"go.uber.org/zap"
	"golang.org/x/mod/module"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Gateway struct {
	GatewayOptions
	config        *config.GatewayConfig
	tlsConfig     *tls.Config
	logger        *zap.SugaredLogger
	httpServer    *GatewayHTTPServer
	grpcServer    *GatewayGRPCServer
	statusQuerier health.HealthStatusQuerier

	storageBackend  storage.Backend
	capBackendStore capabilities.BackendStore
}

type GatewayOptions struct {
	lifecycler config.Lifecycler
}

type GatewayOption func(*GatewayOptions)

func (o *GatewayOptions) apply(opts ...GatewayOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithLifecycler(lc config.Lifecycler) GatewayOption {
	return func(o *GatewayOptions) {
		o.lifecycler = lc
	}
}

func NewGateway(ctx context.Context, conf *config.GatewayConfig, pl plugins.LoaderInterface, opts ...GatewayOption) *Gateway {
	options := GatewayOptions{
		lifecycler: config.NewUnavailableLifecycler(cfgmeta.ObjectList{conf}),
	}
	options.apply(opts...)

	lg := logger.New().Named("gateway")
	conf.Spec.SetDefaults()

	storageBackend, err := machinery.ConfigureStorageBackend(ctx, &conf.Spec.Storage)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Error("failed to configure storage backend")
	}

	// configure the server-side installer template with the external hostname
	// and grpc port which agents will connect to
	_, port, err := net.SplitHostPort(conf.Spec.GRPCListenAddress)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to parse listen address")
	}
	capBackendStore := capabilities.NewBackendStore(capabilities.ServerInstallerTemplateSpec{
		Address: "https://" + conf.Spec.Hostname + ":" + port,
	}, lg)

	// add capabilities from plugins
	pl.Hook(hooks.OnLoadM(func(p types.CapabilityBackendPlugin, md meta.PluginMeta) {
		info, err := p.Info(ctx, &emptypb.Empty{})
		if err != nil {
			lg.With(
				zap.String("plugin", md.Module),
			).Error("failed to get capability info")
			return
		}
		backend := capabilities.NewBackend(p)
		if err := capBackendStore.Add(info.CapabilityName, backend); err != nil {
			lg.With(
				zap.String("plugin", md.Module),
				zap.Error(err),
			).Error("failed to add capability backend")
		}
	}))

	// serve system plugin kv stores
	pl.Hook(hooks.OnLoadM(func(p types.SystemPlugin, md meta.PluginMeta) {
		ns := md.Module
		if err := module.CheckPath(ns); err != nil {
			lg.With(
				zap.String("namespace", ns),
				zap.Error(err),
			).Warn("system plugin module name is invalid")
			return
		}
		store, err := storageBackend.KeyValueStore(ns)
		if err != nil {
			lg.With(
				zap.String("namespace", ns),
				zap.Error(err),
			).Error("failed to get key value store for plugin")
		}
		go p.ServeKeyValueStore(store)
	}))

	// set up http server
	tlsConfig, pkey, err := loadTLSConfig(&conf.Spec)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to load TLS config")
	}

	httpServer := NewHTTPServer(ctx, &conf.Spec, lg, pl)

	clusterAuth, err := cluster.New(ctx, storageBackend, auth.AuthorizationKey,
		cluster.WithExcludeGRPCMethodsFromAuth("/bootstrap.Bootstrap/Join", "/bootstrap.Bootstrap/Auth"),
	)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to create cluster auth")
	}

	// set up grpc server
	grpcServer := NewGRPCServer(&conf.Spec, lg,
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.ChainStreamInterceptor(clusterAuth.StreamServerInterceptor()),
		grpc.ChainUnaryInterceptor(clusterAuth.UnaryServerInterceptor()),
	)

	// set up stream server
	listener := health.NewListener()
	monitor := health.NewMonitor(health.WithLogger(lg.Named("monitor")))
	go monitor.Run(ctx, listener)
	streamSvc := NewStreamServer(listener, lg)
	streamv1.RegisterStreamServer(grpcServer, streamSvc)

	pl.Hook(hooks.OnLoadMC(func(ext types.StreamAPIExtensionPlugin, md meta.PluginMeta, cc *grpc.ClientConn) {
		services, err := ext.Services(ctx, &emptypb.Empty{})
		if err != nil {
			lg.With(
				zap.Error(err),
				"plugin", md.Module,
			).Error("failed to load stream services from plugin")
		}
		if err := streamSvc.AddRemote(cc, services.Descriptors); err != nil {
			lg.With(
				zap.Error(err),
				"plugin", md.Module,
			).Error("failed to add plugin remote stream service")
		}
	}))

	// set up bootstrap server
	bootstrapSvc := bootstrap.NewServer(storageBackend, pkey, capBackendStore)
	bootstrapv1.RegisterBootstrapServer(grpcServer, bootstrapSvc)

	//set up unary plugins
	unarySvc := NewUnaryService()
	unarySvc.RegisterUnaryPlugins(ctx, grpcServer, pl)

	g := &Gateway{
		GatewayOptions:  options,
		tlsConfig:       tlsConfig,
		config:          conf,
		logger:          lg,
		storageBackend:  storageBackend,
		capBackendStore: capBackendStore,
		httpServer:      httpServer,
		grpcServer:      grpcServer,
		statusQuerier:   monitor,
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

func (g *Gateway) ListenAndServe(ctx context.Context) error {
	lg := g.logger

	// start http server
	waitctx.Go(ctx, func() {
		if err := g.httpServer.ListenAndServe(ctx); err != nil {
			lg.With(
				zap.Error(err),
			).Warn("http server exited with error")
		}
	})

	// start grpc server
	waitctx.Go(ctx, func() {
		if err := g.grpcServer.ListenAndServe(ctx); err != nil {
			lg.With(
				zap.Error(err),
			).Warn("grpc server exited with error")
		}
	})

	// start web server
	webuiSrv, err := webui.NewWebUIServer(g.config)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Warn("not starting web ui server")
	} else {
		waitctx.Go(ctx, func() {
			if err := webuiSrv.ListenAndServe(ctx); err != nil {
				lg.With(
					zap.Error(err),
				).Warn("ui server exited with error")
			}
		})
	}

	<-ctx.Done()
	waitctx.Wait(ctx)
	return ctx.Err()
}

// Implements management.CoreDataSource
func (g *Gateway) StorageBackend() storage.Backend {
	return g.storageBackend
}

// Implements management.CoreDataSource
func (g *Gateway) TLSConfig() *tls.Config {
	return g.httpServer.tlsConfig
}

// Implements management.CapabilitiesDataSource
func (g *Gateway) CapabilitiesStore() capabilities.BackendStore {
	return g.capBackendStore
}

// Implements management.HealthStatusDataSource
func (g *Gateway) ClusterHealthStatus(ref *corev1.Reference) (*corev1.HealthStatus, error) {
	hs := g.statusQuerier.GetHealthStatus(ref.Id)
	if hs.Health == nil && hs.Status == nil {
		return nil, status.Error(codes.NotFound, "no health or status has been reported for this cluster yet")
	}
	return hs, nil
}

func (g *Gateway) MustRegisterCollector(collector prometheus.Collector) {
	g.httpServer.metricsHandler.MustRegister(collector)
}

func loadTLSConfig(cfg *v1beta1.GatewayConfigSpec) (*tls.Config, crypto.Signer, error) {
	servingCertBundle, caPool, err := util.LoadServingCertBundle(cfg.Certs)
	if err != nil {
		return nil, nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      caPool,
		Certificates: []tls.Certificate{*servingCertBundle},
	}, servingCertBundle.PrivateKey.(crypto.Signer), nil
}
