package gateway

import (
	"context"
	"crypto"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/rancher/opni/pkg/auth/challenges"
	authv1 "github.com/rancher/opni/pkg/auth/cluster/v1"
	authv2 "github.com/rancher/opni/pkg/auth/cluster/v2"
	"github.com/rancher/opni/pkg/update"
	k8sserver "github.com/rancher/opni/pkg/update/kubernetes/server"
	patchserver "github.com/rancher/opni/pkg/update/patch/server"
	"github.com/spf13/afero"

	"slices"

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
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/auth/session"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/config"
	cfgmeta "github.com/rancher/opni/pkg/config/meta"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/plugins/types"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
)

type Gateway struct {
	GatewayOptions
	config            *config.GatewayConfig
	tlsConfig         *tls.Config
	logger            *zap.SugaredLogger
	httpServer        *GatewayHTTPServer
	grpcServer        *GatewayGRPCServer
	connectionTracker *ConnectionTracker
	delegate          *DelegateServer
	statusQuerier     health.HealthStatusQuerier

	storageBackend  storage.Backend
	capBackendStore capabilities.BackendStore
}

type GatewayOptions struct {
	lifecycler          config.Lifecycler
	extraUpdateHandlers []update.UpdateTypeHandler
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

func WithExtraUpdateHandlers(handlers ...update.UpdateTypeHandler) GatewayOption {
	return func(o *GatewayOptions) {
		o.extraUpdateHandlers = append(o.extraUpdateHandlers, handlers...)
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
		).Panic("failed to configure storage backend")
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

	// serve caching provider for plugin RPCs
	pl.Hook(hooks.OnLoadM(func(p types.SystemPlugin, md meta.PluginMeta) {
		ns := md.Module
		if err := module.CheckPath(ns); err != nil {
			lg.With(
				zap.String("namespace", ns),
				zap.Error(err),
			).Warn("system plugin module name is invalid")
			return
		}
		go p.ServeCachingProvider()
	}))

	// set up http server
	httpServer := NewHTTPServer(ctx, &conf.Spec, lg, pl)

	// Set up cluster auth
	ephemeralKeys, err := machinery.LoadEphemeralKeys(afero.Afero{
		Fs: afero.NewOsFs(),
	}, conf.Spec.Keyring.EphemeralKeyDirs...)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Panic("failed to load ephemeral keys")
	}

	v1Verifier := challenges.NewKeyringVerifier(storageBackend, authv1.DomainString, lg.Named("authv1"))
	v2Verifier := challenges.NewKeyringVerifier(storageBackend, authv2.DomainString, lg.Named("authv2"))

	sessionAttrChallenge, err := session.NewServerChallenge(
		keyring.New(lo.ToAnySlice(ephemeralKeys)...))
	if err != nil {
		lg.With(
			zap.Error(err),
		).Panic("failed to configure authentication")
	}

	v1Challenge := authv1.NewServerChallenge(streamv1.Stream_Connect_FullMethodName, v1Verifier, lg.Named("authv1"))
	v2Challenge := authv2.NewServerChallenge(v2Verifier, lg.Named("authv2"))

	clusterAuth := cluster.StreamServerInterceptor(challenges.Chained(
		challenges.If(authv2.ShouldEnableIncoming).Then(v2Challenge).Else(v1Challenge),
		challenges.If(session.ShouldEnableIncoming).Then(sessionAttrChallenge),
	))

	//set up update server
	updateServer := update.NewUpdateServer(lg)

	// set up plugin sync server
	binarySyncServer, err := patchserver.NewFilesystemPluginSyncServer(conf.Spec.Plugins, lg,
		patchserver.WithPluginSyncFilters(func(pm meta.PluginMeta) bool {
			if pm.ExtendedMetadata != nil {
				// only sync plugins that have the agent mode set
				return slices.Contains(pm.ExtendedMetadata.ModeList.Modes, meta.ModeAgent)
			}
			return true // default to syncing all plugins
		}),
	)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Panic("failed to create plugin sync server")
	}

	if err := binarySyncServer.RunGarbageCollection(ctx, storageBackend); err != nil {
		lg.With(
			zap.Error(err),
		).Error("failed to run garbage collection")
	}

	updateServer.RegisterUpdateHandler(binarySyncServer.Strategy(), binarySyncServer)

	kubernetesSyncServer, err := k8sserver.NewKubernetesSyncServer(conf.Spec.AgentUpgrades.Kubernetes, lg)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Panic("failed to create kubernetes agent sync server")
	}
	updateServer.RegisterUpdateHandler(kubernetesSyncServer.Strategy(), kubernetesSyncServer)

	for _, handler := range options.extraUpdateHandlers {
		updateServer.RegisterUpdateHandler(handler.Strategy(), handler)
	}

	httpServer.metricsRegisterer.MustRegister(updateServer.Collectors()...)

	// set up grpc server
	tlsConfig, pkey, err := grpcTLSConfig(&conf.Spec)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Panic("failed to load TLS config")
	}

	rateLimitOpts := []RatelimiterOption{}
	if conf.Spec.RateLimit != nil {
		rateLimitOpts = append(rateLimitOpts, WithRate(conf.Spec.RateLimit.Rate))
		rateLimitOpts = append(rateLimitOpts, WithBurst(conf.Spec.RateLimit.Burst))
	}

	// set up stream server
	listener := health.NewListener()
	buffer := health.NewBuffer()

	// set up connection tracker
	var connectionTracker *ConnectionTracker
	if conf.Spec.Management.RelayListenAddress != "" {
		// require a lock manager
		storageBackendLmBroker, ok := storageBackend.(storage.LockManagerBroker)
		if !ok || storageBackendLmBroker == nil {
			lg.With(
				zap.Error(err),
			).Panic("storage backend does not support distributed locking and cannot be used if the relay server is configured")
		}
		lg := lg.Named("connections")
		connectionsKv := storageBackend.KeyValueStore("connections")
		connectionsLm := storageBackendLmBroker.LockManager("connections")
		advertiseAddr := conf.Spec.Management.RelayAdvertiseAddress
		if advertiseAddr == "" {
			lg.Warn("relay advertise address not set; will advertise the listen address")
			advertiseAddr = conf.Spec.Management.RelayListenAddress
		}
		connectionTracker = NewConnectionTracker(ctx, &corev1.InstanceInfo{
			RelayAddress:      os.ExpandEnv(conf.Spec.Management.RelayAdvertiseAddress),
			ManagementAddress: conf.Spec.Management.GRPCListenAddress,
		}, connectionsKv, connectionsLm, lg)

		writerManager := NewHealthStatusWriterManager(ctx, connectionsKv, lg)
		go health.Copy(ctx, writerManager, listener)
		// localUpdater := health.NewFanOutUpdater(listener, writerManager)
		connectionTracker.AddTrackedConnectionListener(writerManager)

		kvReader, err := NewHealthStatusReader(ctx, connectionsKv)
		if err != nil {
			lg.With(
				zap.Error(err),
			).Panic("failed to create health status reader")
		}
		go health.Copy(ctx, buffer, kvReader)
	} else {
		go health.Copy(ctx, buffer, listener)
	}

	monitor := health.NewMonitor(health.WithLogger(lg.Named("monitor")))
	delegateServerOpts := []DelegateServerOption{}
	if connectionTracker != nil {
		delegateServerOpts = append(delegateServerOpts, WithConnectionTracker(connectionTracker))
	}
	delegate := NewDelegateServer(conf.Spec, storageBackend, lg, delegateServerOpts...)
	agentHandler := MultiConnectionHandler(listener, delegate)

	go monitor.Run(ctx, buffer)

	// initialize grpc server

	var streamInterceptors []grpc.StreamServerInterceptor
	streamInterceptors = append(streamInterceptors, NewRateLimiterInterceptor(lg, rateLimitOpts...).StreamServerInterceptor())
	streamInterceptors = append(streamInterceptors, clusterAuth)
	if connectionTracker != nil {
		streamInterceptors = append(streamInterceptors, connectionTracker.StreamServerInterceptor())
	}
	streamInterceptors = append(streamInterceptors, updateServer.StreamServerInterceptor())
	streamInterceptors = append(streamInterceptors, NewLastKnownDetailsApplier(storageBackend))

	grpcServer := NewGRPCServer(&conf.Spec, lg,
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	)

	streamSvc := NewStreamServer(agentHandler, storageBackend, httpServer.metricsRegisterer, lg)
	controlv1.RegisterHealthListenerServer(streamSvc, listener)
	streamv1.RegisterDelegateServer(streamSvc.InternalServiceRegistrar(), delegate)
	streamv1.RegisterStreamServer(grpcServer, streamSvc)
	corev1.RegisterPingerServer(streamSvc, &pinger{})
	controlv1.RegisterUpdateSyncServer(grpcServer, updateServer)

	pl.Hook(hooks.OnLoadMC(streamSvc.OnPluginLoad))

	// set up bootstrap server
	bootstrapServerV1 := bootstrap.NewServer(storageBackend, pkey, capBackendStore)
	bootstrapServerV2 := bootstrap.NewServerV2(storageBackend, pkey)
	bootstrapv1.RegisterBootstrapServer(grpcServer, bootstrapServerV1)
	bootstrapv2.RegisterBootstrapServer(grpcServer, bootstrapServerV2)

	g := &Gateway{
		GatewayOptions:    options,
		tlsConfig:         tlsConfig,
		config:            conf,
		logger:            lg,
		storageBackend:    storageBackend,
		capBackendStore:   capBackendStore,
		httpServer:        httpServer,
		grpcServer:        grpcServer,
		statusQuerier:     monitor,
		connectionTracker: connectionTracker,
		delegate:          delegate,
	}

	waitctx.Go(ctx, func() {
		<-ctx.Done()
		storageBackend.Close()
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
	ctx, ca := context.WithCancelCause(ctx)

	channels := []<-chan error{
		// start http server
		lo.Async(func() error {
			err := g.httpServer.ListenAndServe(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					lg.Info("http server stopped")
				} else {
					lg.With(zap.Error(err)).Warn("http server exited with error")
				}
			}
			return err
		}),
		// start grpc server
		lo.Async(func() error {
			err := g.grpcServer.ListenAndServe(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					lg.Info("grpc server stopped")
				} else {
					lg.With(zap.Error(err)).Warn("grpc server exited with error")
				}
			}
			return err
		}),
	}

	if g.connectionTracker != nil {
		channels = append(channels, lo.Async(func() error {
			err := g.connectionTracker.Run(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					lg.Info("connection tracker stopped")
				} else {
					lg.With(zap.Error(err)).Warn("connection tracker exited with error")
				}
			}
			return err
		}))
		channels = append(channels, lo.Async(func() error {
			relaySrv := g.delegate.NewRelayServer()
			err := relaySrv.ListenAndServe(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					lg.Info("relay server stopped")
				} else {
					lg.With(zap.Error(err)).Warn("relay server exited with error")
				}
			}
			return err
		}))
	}

	return util.WaitAll(ctx, ca, channels...)
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

func grpcTLSConfig(cfg *v1beta1.GatewayConfigSpec) (*tls.Config, crypto.Signer, error) {
	servingCertBundle, caPool, err := util.LoadServingCertBundle(cfg.Certs)
	if err != nil {
		return nil, nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		RootCAs:      caPool,
		Certificates: []tls.Certificate{*servingCertBundle},
		ClientAuth:   tls.NoClientCert,
	}, servingCertBundle.PrivateKey.(crypto.Signer), nil
}

func httpTLSConfig(cfg *v1beta1.GatewayConfigSpec) (*tls.Config, crypto.Signer, error) {
	servingCertBundle, caPool, err := util.LoadServingCertBundle(cfg.Certs)
	if err != nil {
		return nil, nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      caPool,
		ClientCAs:    caPool,
		Certificates: []tls.Certificate{*servingCertBundle},
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}, servingCertBundle.PrivateKey.(crypto.Signer), nil
}
