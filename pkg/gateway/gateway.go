package gateway

import (
	"context"
	"crypto"
	"crypto/tls"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"

	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	bootstrapv2 "github.com/rancher/opni/pkg/apis/bootstrap/v2"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/auth/challenges"
	"github.com/rancher/opni/pkg/auth/cluster"
	authv1 "github.com/rancher/opni/pkg/auth/cluster/v1"
	authv2 "github.com/rancher/opni/pkg/auth/cluster/v2"
	"github.com/rancher/opni/pkg/auth/session"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/config/reactive"
	"github.com/rancher/opni/pkg/config/reactive/subtle"
	configv1 "github.com/rancher/opni/pkg/config/v1"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/management"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/plugins/types"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/update"
	k8sserver "github.com/rancher/opni/pkg/update/kubernetes/server"
	"github.com/rancher/opni/pkg/update/patch"
	patchserver "github.com/rancher/opni/pkg/update/patch/server"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"github.com/spf13/afero"
	"golang.org/x/mod/module"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Gateway struct {
	GatewayOptions
	mgr               *configv1.GatewayConfigManager
	certs             reactive.Reactive[*configv1.CertsSpec]
	httpServer        *GatewayHTTPServer
	grpcServer        *GatewayGRPCServer
	connectionTracker *ConnectionTracker
	delegate          *DelegateServer
	statusQuerier     health.HealthStatusQuerier

	storageBackend storage.Backend
	capDataSource  *capabilitiesDataSource
}

type GatewayOptions struct {
	logger              *slog.Logger
	extraUpdateHandlers []update.UpdateTypeHandler
}

type GatewayOption func(*GatewayOptions)

func (o *GatewayOptions) apply(opts ...GatewayOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithExtraUpdateHandlers(handlers ...update.UpdateTypeHandler) GatewayOption {
	return func(o *GatewayOptions) {
		o.extraUpdateHandlers = append(o.extraUpdateHandlers, handlers...)
	}
}

func WithLogger(logger *slog.Logger) GatewayOption {
	return func(o *GatewayOptions) {
		o.logger = logger
	}
}

func NewGateway(
	ctx context.Context,
	mgr *configv1.GatewayConfigManager,
	storageBackend storage.Backend,
	pl plugins.LoaderInterface,
	opts ...GatewayOption,
) *Gateway {
	options := GatewayOptions{
		logger: logger.New().WithGroup("gateway"),
	}
	options.apply(opts...)

	lg := options.logger

	capBackendStore := capabilities.NewBackendStore(lg)

	// add capabilities from plugins
	pl.Hook(hooks.OnLoadM(func(p types.CapabilityBackendPlugin, md meta.PluginMeta) {
		list, err := p.List(ctx, &emptypb.Empty{})
		if err != nil {
			lg.With(
				"plugin", md.Module,
				logger.Err(err),
			).Error("failed to list capabilities")
			return
		}
		for _, cap := range list.GetItems() {
			info, err := p.Info(ctx, &corev1.Reference{Id: cap.GetName()})
			if err != nil {
				lg.With(
					"plugin", md.Module,
				).Error("failed to get capability info")
				return
			}
			if err := capBackendStore.Add(info.Name, p); err != nil {
				lg.With(
					"plugin", md.Module,
					logger.Err(err),
				).Error("failed to add capability backend")
			}
			lg.With(
				"plugin", md.Module,
				"capability", info.Name,
			).Info("added capability backend")
		}
	}))

	// serve system plugin kv stores
	pl.Hook(hooks.OnLoadM(func(p types.SystemPlugin, md meta.PluginMeta) {
		ns := md.Module
		if err := module.CheckPath(ns); err != nil {
			lg.With(
				"namespace", ns,
				logger.Err(err),
			).Warn("system plugin module name is invalid")
			return
		}
		go p.ServeKeyValueStore(ns, storageBackend)
	}))

	// serve caching provider for plugin RPCs
	pl.Hook(hooks.OnLoadM(func(p types.SystemPlugin, md meta.PluginMeta) {
		ns := md.Module
		if err := module.CheckPath(ns); err != nil {
			lg.With(
				"namespace", ns,
				logger.Err(err),
			).Warn("system plugin module name is invalid")
			return
		}
		go p.ServeCachingProvider()
	}))

	// set up http server
	httpServer, err := NewHTTPServer(ctx, mgr, lg, pl)
	if err != nil {
		panic(fmt.Errorf("failed to create http server: %w", err))
	}

	runtimeKeyDirList := subtle.WaitOne(ctx, mgr.Reactive(configv1.ProtoPath().Keyring().RuntimeKeyDirs())).List()
	runtimeKeyDirs := make([]string, 0, runtimeKeyDirList.Len())
	for i, l := 0, runtimeKeyDirList.Len(); i < l; i++ {
		runtimeKeyDirs = append(runtimeKeyDirs, runtimeKeyDirList.Get(i).String())
	}

	// Set up cluster auth
	ephemeralKeys, err := machinery.LoadEphemeralKeys(afero.Afero{
		Fs: afero.NewOsFs(),
	}, runtimeKeyDirs...)
	if err != nil {
		lg.With(
			logger.Err(err),
		).Error("failed to load ephemeral keys")
		panic("failed to load ephemeral keys")
	}

	v1Verifier := challenges.NewKeyringVerifier(storageBackend, authv1.DomainString, lg.WithGroup("authv1"))
	v2Verifier := challenges.NewKeyringVerifier(storageBackend, authv2.DomainString, lg.WithGroup("authv2"))

	sessionAttrChallenge, err := session.NewServerChallenge(
		keyring.New(lo.ToAnySlice(ephemeralKeys)...))
	if err != nil {
		lg.With(
			logger.Err(err),
		).Error("failed to configure authentication")
		panic("failed to configure authentication")
	}

	v1Challenge := authv1.NewServerChallenge(streamv1.Stream_Connect_FullMethodName, v1Verifier, lg.WithGroup("authv1"))
	v2Challenge := authv2.NewServerChallenge(v2Verifier, lg.WithGroup("authv2"))

	clusterAuth := cluster.StreamServerInterceptor(challenges.Chained(
		challenges.If(authv2.ShouldEnableIncoming).Then(v2Challenge).Else(v1Challenge),
		challenges.If(session.ShouldEnableIncoming).Then(sessionAttrChallenge),
	))

	//set up update server
	updateServer := update.NewUpdateServer(lg)

	// set up plugin sync server
	pluginConfig := subtle.WaitOneMessage[*configv1.PluginsSpec](ctx, mgr.Reactive(protopath.Path(configv1.ProtoPath().Plugins())))
	patchEngine := patch.NewReactivePatchEngine(ctx, lg, mgr.Reactive(configv1.ProtoPath().Upgrades().Plugins().Binary().PatchEngine()))

	binarySyncServer, err := patchserver.NewFilesystemPluginSyncServer(ctx, pluginConfig.Cache, patchEngine, lg,
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
			logger.Err(err),
		).Error("failed to create plugin sync server")
		panic("failed to create plugin sync server")
	}

	if err := binarySyncServer.RunGarbageCollection(ctx, storageBackend); err != nil {
		lg.With(
			logger.Err(err),
		).Error("failed to run garbage collection")
	}

	updateServer.RegisterUpdateHandler(binarySyncServer.Strategy(), binarySyncServer)

	k8sSyncServerConfig := reactive.Message[*configv1.KubernetesAgentUpgradeSpec](
		mgr.Reactive(protopath.Path(configv1.ProtoPath().Upgrades().Agents().Kubernetes())))
	kubernetesSyncServer, err := k8sserver.NewKubernetesSyncServer(ctx, k8sSyncServerConfig, lg)
	if err != nil {
		lg.With(
			logger.Err(err),
		).Error("failed to create kubernetes agent sync server")
		// panic("failed to create kubernetes agent sync server")
	} else {
		updateServer.RegisterUpdateHandler(kubernetesSyncServer.Strategy(), kubernetesSyncServer)
	}

	for _, handler := range options.extraUpdateHandlers {
		updateServer.RegisterUpdateHandler(handler.Strategy(), handler)
	}

	httpServer.metricsRegisterer.MustRegister(updateServer.Collectors()...)

	// set up grpc server
	// tlsConfig, pkey, err := grpcTLSConfig(&conf.Spec)
	// if err != nil {
	// 	lg.With(
	// 		logger.Err(err),
	// 	).Error("failed to load TLS config")
	// 	panic("failed to load TLS config")
	// }

	// rateLimitOpts := []RatelimiterOption{}
	// if conf.RateLimiting != nil {
	// 	rateLimitOpts = append(rateLimitOpts, WithRate(conf.RateLimiting.GetRate()))
	// 	rateLimitOpts = append(rateLimitOpts, WithBurst(int(conf.RateLimiting.GetBurst())))
	// }

	// set up stream server
	listener := health.NewListener()
	buffer := health.NewBuffer()

	// set up connection tracker
	var connectionTracker *ConnectionTracker
	{
		storageBackendLmBroker := storageBackend.(storage.LockManagerBroker)
		lg := lg.WithGroup("connections")
		connectionsKv := storageBackend.KeyValueStore("connections")
		connectionsLm := storageBackendLmBroker.LockManager("connections")

		connectionTracker = NewConnectionTracker(ctx, mgr, connectionsKv, connectionsLm, lg)

		writerManager := NewHealthStatusWriterManager(ctx, connectionsKv, lg)
		go health.Copy(ctx, writerManager, listener)
		// localUpdater := health.NewFanOutUpdater(listener, writerManager)
		connectionTracker.AddTrackedConnectionListener(writerManager)

		kvReader, err := NewHealthStatusReader(ctx, connectionsKv)
		if err != nil {
			panic(fmt.Errorf("failed to create health status reader: %w", err))
		}
		go health.Copy(ctx, buffer, kvReader)
	}

	monitor := health.NewMonitor(health.WithLogger(lg.WithGroup("monitor")))
	delegateServerOpts := []DelegateServerOption{}
	if connectionTracker != nil {
		delegateServerOpts = append(delegateServerOpts, WithConnectionTracker(connectionTracker))
	}
	delegate := NewDelegateServer(mgr, storageBackend, lg, delegateServerOpts...)
	agentHandler := MultiConnectionHandler(listener, delegate)

	capDataSource := &capabilitiesDataSource{
		capBackendStore: capBackendStore,
		delegate:        delegate,
		logger:          lg.WithGroup("capabilities"),
	}

	go monitor.Run(ctx, buffer)

	// initialize grpc server

	var streamInterceptors []grpc.StreamServerInterceptor
	rateLimitConfig := reactive.Message[*configv1.RateLimitingSpec](mgr.Reactive(protopath.Path(configv1.ProtoPath().RateLimiting())))
	streamInterceptors = append(streamInterceptors, NewRateLimiterInterceptor(ctx, lg, rateLimitConfig).StreamServerInterceptor())
	streamInterceptors = append(streamInterceptors, clusterAuth)
	if connectionTracker != nil {
		streamInterceptors = append(streamInterceptors, connectionTracker.StreamServerInterceptor())
	}
	streamInterceptors = append(streamInterceptors, updateServer.StreamServerInterceptor())
	streamInterceptors = append(streamInterceptors, NewLastKnownDetailsApplier(storageBackend))

	grpcServer := NewGRPCServer(mgr, lg,
		grpc.ChainStreamInterceptor(streamInterceptors...),
	)

	streamSvc := NewStreamServer(agentHandler, storageBackend, httpServer.metricsRegisterer, lg)
	controlv1.RegisterHealthListenerServer(streamSvc, listener)
	streamv1.RegisterDelegateServer(streamSvc.InternalServiceRegistrar(), delegate)
	streamv1.RegisterStreamServer(grpcServer, streamSvc)
	corev1.RegisterPingerServer(streamSvc, &pinger{})
	controlv1.RegisterUpdateSyncServer(grpcServer, updateServer)

	pl.Hook(hooks.OnLoadMC(streamSvc.OnPluginLoad))

	certs := reactive.Message[*configv1.CertsSpec](mgr.Reactive(protopath.Path(configv1.ProtoPath().Certs())))
	// set up bootstrap server
	pkey := &atomic.Pointer[crypto.Signer]{}
	certs.WatchFunc(ctx, func(value *configv1.CertsSpec) {
		tlsConfig, _ := value.AsTlsConfig(tls.NoClientCert)
		signer := tlsConfig.Certificates[0].PrivateKey.(crypto.Signer)
		pkey.Store(&signer)
	})
	bootstrapServerV2 := bootstrap.NewServerV2(bootstrap.NewStorage(storageBackend), pkey)
	bootstrapv2.RegisterBootstrapServer(grpcServer, bootstrapServerV2)

	g := &Gateway{
		certs:             certs,
		mgr:               mgr,
		GatewayOptions:    options,
		storageBackend:    storageBackend,
		capDataSource:     capDataSource,
		httpServer:        httpServer,
		grpcServer:        grpcServer,
		statusQuerier:     monitor,
		connectionTracker: connectionTracker,
		delegate:          delegate,
	}

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
					lg.With(logger.Err(err)).Warn("http server exited with error")
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
					lg.With(logger.Err(err)).Warn("grpc server exited with error")
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
					lg.With(logger.Err(err)).Warn("connection tracker exited with error")
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
					lg.With(logger.Err(err)).Warn("relay server exited with error")
				}
			}
			return err
		}))
	}

	util.WaitAll(ctx, ca, channels...)
	return context.Cause(ctx)
}

// Implements management.CoreDataSource
func (g *Gateway) StorageBackend() storage.Backend {
	return g.storageBackend
}

// Implements management.CoreDataSource
func (g *Gateway) TLSConfig() *tls.Config {
	tc, err := g.certs.Value().AsTlsConfig(tls.NoClientCert)
	if err != nil {
		return nil
	}
	return tc
}

// Implements management.CapabilitiesDataSource
func (g *Gateway) CapabilitiesDataSource() management.CapabilitiesDataSource {
	return g.capDataSource
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
