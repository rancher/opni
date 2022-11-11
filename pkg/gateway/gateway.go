package gateway

import (
	"context"
	"crypto"
	"crypto/tls"
	"fmt"
	"net"

	"github.com/rancher/opni/pkg/patch"

	"github.com/hashicorp/go-plugin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"golang.org/x/mod/module"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	bootstrapv1 "github.com/rancher/opni/pkg/apis/bootstrap/v1"
	bootstrapv2 "github.com/rancher/opni/pkg/apis/bootstrap/v2"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
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
	syncRequester   *SyncRequester
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
		).Panic("failed to parse listen address")
	}
	capBackendStore := capabilities.NewBackendStore(capabilities.ServerInstallerTemplateSpec{
		Address: conf.Spec.Hostname,
		Port:    fmt.Sprint(port),
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
		if err := capBackendStore.Add(info.Name, p); err != nil {
			lg.With(
				zap.String("plugin", md.Module),
				zap.Error(err),
			).Error("failed to add capability backend")
		}
		lg.With(
			zap.String("plugin", md.Module),
			zap.String("capability", info.Name),
		).Info("added capability backend")
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
		store := storageBackend.KeyValueStore(ns)
		go p.ServeKeyValueStore(store)
	}))

	// set up http server
	tlsConfig, pkey, err := loadTLSConfig(&conf.Spec)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Panic("failed to load TLS config")
	}

	httpServer := NewHTTPServer(ctx, &conf.Spec, lg, pl)

	clusterAuth, err := cluster.New(ctx, storageBackend, auth.AuthorizationKey,
		cluster.WithExcludeGRPCMethodsFromAuth(
			"/bootstrap.Bootstrap/Join", "/bootstrap.Bootstrap/Auth",
			"/bootstrap.v2.Bootstrap/Join", "/bootstrap.v2.Bootstrap/Auth",
		),
	)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Panic("failed to create cluster auth")
	}

	// set up plugin manifest server
	manifest := patch.NewFilesystemPluginSyncServer(conf.Spec.Plugins, lg)

	// set up grpc server
	interceptor := clusterAuth.UnaryServerInterceptor()
	grpcServer := NewGRPCServer(&conf.Spec, lg,
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.ChainStreamInterceptor(
			clusterAuth.StreamServerInterceptor(),
			manifest.StreamServerInterceptor(),
		),
		grpc.ChainUnaryInterceptor(interceptor),
	)

	// set up stream server
	listener := health.NewListener()
	monitor := health.NewMonitor(health.WithLogger(lg.Named("monitor")))
	sync := NewSyncRequester(lg)
	// set up agent connection handlers
	agentHandler := MultiConnectionHandler(listener, sync)
	go monitor.Run(ctx, listener)
	streamSvc := NewStreamServer(agentHandler, storageBackend, interceptor, lg)
	controlv1.RegisterHealthListenerServer(streamSvc, listener)

	streamv1.RegisterStreamServer(grpcServer, streamSvc)
	controlv1.RegisterPluginManifestServer(grpcServer, manifest)

	pl.Hook(hooks.OnLoadMC(func(ext types.StreamAPIExtensionPlugin, md meta.PluginMeta, cc *grpc.ClientConn) {
		// services, err := ext.Services(ctx, &emptypb.Empty{})
		// if err != nil {
		// 	lg.With(
		// 		zap.Error(err),
		// 		"plugin", md.Module,
		// 	).Error("failed to load stream services from plugin")
		// }
		if err := streamSvc.AddRemote(cc, md.ShortName()); err != nil {
			lg.With(
				zap.Error(err),
				"plugin", md.Module,
			).Error("failed to add plugin remote stream service")
		}
	}))

	// set up bootstrap server
	bootstrapServerV1 := bootstrap.NewServer(storageBackend, pkey, capBackendStore)
	bootstrapServerV2 := bootstrap.NewServerV2(storageBackend, pkey)
	bootstrapv1.RegisterBootstrapServer(grpcServer, bootstrapServerV1)
	bootstrapv2.RegisterBootstrapServer(grpcServer, bootstrapServerV2)

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
		syncRequester:   sync,
	}

	waitctx.Go(ctx, func() {
		<-ctx.Done()
		lg.Info("shutting down plugins")
		plugin.CleanupClients()
		lg.Info("all plugins shut down")
	})

	return g
}

type keyValueStoreServer interface {
	ServeKeyValueStore(store storage.KeyValueStore)
}

func (g *Gateway) ListenAndServe(ctx context.Context) error {
	lg := g.logger
	ctx, ca := context.WithCancel(ctx)

	// start http server
	e1 := lo.Async(func() error {
		err := g.httpServer.ListenAndServe(ctx)
		if err != nil {
			lg.With(
				zap.Error(err),
			).Warn("http server exited with error")
		}
		return err
	})

	// start grpc server
	e2 := lo.Async(func() error {
		err := g.grpcServer.ListenAndServe(ctx)
		if err != nil {
			lg.With(
				zap.Error(err),
			).Warn("grpc server exited with error")
		}
		return err
	})

	// start web server
	webuiSrv, err := webui.NewWebUIServer(g.config)
	var e3 chan error
	if err != nil {
		lg.With(
			zap.Error(err),
		).Warn("not starting web ui server")
		e3 = make(chan error)
		close(e3)
	} else {
		e3 = lo.Async(func() error {
			err := webuiSrv.ListenAndServe(ctx)
			if err != nil {
				lg.With(
					zap.Error(err),
				).Warn("ui server exited with error")
			}
			return err
		})
	}

	return util.WaitAll(ctx, ca, e1, e2, e3)
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

// Implements management.CapabilitiesDataSource
func (g *Gateway) NodeManagerServer() capabilityv1.NodeManagerServer {
	return g.syncRequester
}

// Implements management.HealthStatusDataSource
func (g *Gateway) GetClusterHealthStatus(ref *corev1.Reference) (*corev1.HealthStatus, error) {
	hs := g.statusQuerier.GetHealthStatus(ref.Id)
	if hs.Health == nil && hs.Status == nil {
		return nil, status.Error(codes.NotFound, "no health or status has been reported for this cluster yet")
	}
	return hs, nil
}

// Implements management.HealthStatusDataSource
func (g *Gateway) WatchClusterHealthStatus(ctx context.Context) <-chan *corev1.ClusterHealthStatus {
	return g.statusQuerier.WatchHealthStatus(ctx)
}

func (g *Gateway) MustRegisterCollector(collector prometheus.Collector) {
	g.httpServer.metricsRegisterer.MustRegister(collector)
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
