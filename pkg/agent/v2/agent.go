package v2

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/health/annotations"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/ident/identserver"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/plugins/types"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/trust"
	"github.com/rancher/opni/pkg/update"
	"github.com/rancher/opni/pkg/urn"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/fwd"
	"github.com/rancher/opni/pkg/versions"
	"github.com/samber/lo"
	"github.com/spf13/afero"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

var ErrRebootstrap = errors.New("re-bootstrap requested")

const (
	healthzPluginsNotLoaded = 1 << iota
	healthzGatewayNotConnected
)

var (
	healthzConditions = map[uint32]string{
		healthzPluginsNotLoaded:    "plugins not loaded",
		healthzGatewayNotConnected: "gateway not connected",
	}
)

type Agent struct {
	AgentOptions

	config       v1beta1.AgentConfigSpec
	router       *gin.Engine
	logger       *slog.Logger
	pluginLoader *plugins.PluginLoader

	tenantID         string
	identityProvider ident.Provider
	keyringStore     storage.KeyringStore
	gatewayClient    clients.GatewayClient
	trust            trust.Strategy
	pluginSyncer     update.SyncHandler
	agentSyncer      update.SyncHandler

	healthzMu *sync.Mutex
	healthz   *uint32

	loadedExistingKeyring bool
}

type AgentOptions struct {
	bootstrapper          bootstrap.Bootstrapper
	unmanagedPluginLoader *plugins.PluginLoader
	rebootstrap           bool
}

type AgentOption func(*AgentOptions)

func (o *AgentOptions) apply(opts ...AgentOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithBootstrapper(bootstrapper bootstrap.Bootstrapper) AgentOption {
	return func(o *AgentOptions) {
		o.bootstrapper = bootstrapper
	}
}

func WithUnmanagedPluginLoader(pluginLoader *plugins.PluginLoader) AgentOption {
	return func(o *AgentOptions) {
		o.unmanagedPluginLoader = pluginLoader
	}
}

func WithRebootstrap(rebootstrap bool) AgentOption {
	return func(o *AgentOptions) {
		o.rebootstrap = rebootstrap
	}
}

func New(ctx context.Context, conf *v1beta1.AgentConfig, opts ...AgentOption) (*Agent, error) {
	options := AgentOptions{}
	options.apply(opts...)
	level := logger.DefaultLogLevel.Level()
	if conf.Spec.LogLevel != "" {
		level = logger.ParseLevel(conf.Spec.LogLevel)
	}
	lg := logger.New(logger.WithLogLevel(level)).Named("agent")
	lg.Debug("using log level:", "level", level.String())

	var pl *plugins.PluginLoader
	if options.unmanagedPluginLoader != nil {
		pl = options.unmanagedPluginLoader
	} else {
		pl = plugins.NewPluginLoader(plugins.WithLogger(lg))
	}

	pl.Hook(hooks.OnLoadM(func(p types.CapabilityNodePlugin, m meta.PluginMeta) {
		lg.Infof("loaded capability node plugin %s", m.Module)
	}))

	router := gin.New()
	routerMutex := &sync.Mutex{}
	router.Use(logger.GinLogger(lg), gin.Recovery())
	pprof.Register(router)

	healthz := new(uint32)
	*healthz = (1 << len(healthzConditions)) - 1
	healthzMu := &sync.Mutex{}

	router.GET("/healthz", func(c *gin.Context) {
		healthzMu.Lock()
		messages := []string{}
		for k, v := range healthzConditions {
			if *healthz&k != 0 {
				messages = append(messages, v)
			}
		}
		healthzMu.Unlock()
		if len(messages) > 0 {
			c.String(http.StatusServiceUnavailable, strings.Join(messages, ", "))
			return
		}
		c.String(http.StatusOK, "OK")
	})

	pl.Hook(hooks.OnLoadingCompleted(func(i int) {
		healthzMu.Lock()
		*healthz &^= healthzPluginsNotLoaded
		healthzMu.Unlock()
	}))

	pl.Hook(hooks.OnLoadM(func(p types.HTTPAPIExtensionPlugin, md meta.PluginMeta) {
		ctx, ca := context.WithTimeout(ctx, 10*time.Second)
		defer ca()
		cfg, err := p.Configure(ctx, apiextensions.NewInsecureCertConfig())
		if err != nil {
			lg.With(
				zap.String("plugin", md.Module),
				zap.Error(err),
			).Error("failed to configure routes")
			return
		}
		setupPluginRoutes(lg, routerMutex, router, cfg, md, []string{"/healthz", "/metrics"})
	}))

	pluginUpgrader, err := machinery.ConfigurePluginUpgrader(
		conf.Spec.PluginUpgrade,
		conf.Spec.PluginDir,
		lg.Named("plugin-upgrader"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to configure plugin syncer: %w", err)
	}

	upgrader, err := machinery.ConfigureAgentUpgrader(
		&conf.Spec.Upgrade,
		lg.Named("agent-upgrader"),
	)
	if err != nil {
		return nil, fmt.Errorf("agent upgrade configuration error: %w", err)
	}

	initCtx, initCancel := context.WithTimeout(ctx, 10*time.Second)
	defer initCancel()

	ipBuilder, err := ident.GetProviderBuilder(conf.Spec.IdentityProvider)
	if err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}
	ip := ipBuilder()
	id, err := ip.UniqueIdentifier(initCtx)
	if err != nil {
		return nil, fmt.Errorf("error getting unique identifier: %w", err)
	}

	sb, err := machinery.ConfigureStorageBackend(initCtx, &conf.Spec.Storage)
	if err != nil {
		return nil, fmt.Errorf("error configuring keyring store broker: %w", err)
	}
	broker, ok := sb.(storage.KeyringStoreBroker)
	if !ok {
		return nil, fmt.Errorf("selected storage backend does not implement storage.KeyringStoreBroker")
	}
	ks := broker.KeyringStore("agent", &corev1.Reference{
		Id: id,
	})

	var kr keyring.Keyring
	var loadedExistingKeyring bool
	var shouldBootstrap bool
	if existing, err := ks.Get(initCtx); err == nil {
		lg.Info("loaded existing keyring")
		kr = existing
		loadedExistingKeyring = true
	} else if storage.IsNotFound(err) {
		lg.Info("no existing keyring found, starting bootstrap process")
		shouldBootstrap = true
	} else {
		return nil, fmt.Errorf("keyring store error: %w", err)
	}

	if options.rebootstrap {
		if conf.Spec.ContainsBootstrapCredentials() {
			lg.Info("attempting to re-bootstrap agent to generate new keyring")
			shouldBootstrap = true
		} else {
			lg.Warn("re-bootstrap requested, but no bootstrap credentials were provided in the config file")
		}
	}

	if shouldBootstrap {
		if options.bootstrapper == nil {
			return nil, fmt.Errorf("bootstrap is required, but no bootstrap configuration was provided")
		}
		kr, err = options.bootstrapper.Bootstrap(initCtx, ip)
		if err != nil {
			return nil, fmt.Errorf("error during bootstrap: %w", err)
		}
		for {
			// Don't let this fail easily, otherwise we will lose the keyring forever.
			// Keep retrying until it succeeds.
			err = ks.Put(ctx, kr)
			if err != nil {
				lg.With(zap.Error(err)).Error("failed to persist keyring (retry in 1 second)")
				time.Sleep(1 * time.Second)
			} else {
				if options.rebootstrap {
					lg.Info("successfully replaced keyring")
				}
				break
			}
		}
		lg.Info("bootstrap completed successfully")
	}

	trust, err := machinery.BuildTrustStrategy(conf.Spec.TrustStrategy, kr)
	if err != nil {
		return nil, fmt.Errorf("error building trust strategy: %w", err)
	}

	// Load ephemeral keyrings from disk, if any search dirs are configured
	ekeys, err := machinery.LoadEphemeralKeys(afero.Afero{
		Fs: afero.NewOsFs(),
	}, conf.Spec.Keyring.EphemeralKeyDirs...)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Warn("error loading ephemeral keys")
	} else if len(ekeys) > 0 {
		kr = kr.Merge(keyring.New(lo.ToAnySlice(ekeys)...))
	}

	gatewayClient, err := clients.NewGatewayClient(ctx,
		conf.Spec.GatewayAddress, ip, kr, trust)
	if err != nil {
		return nil, fmt.Errorf("error configuring gateway client: %w", err)
	}
	controlv1.RegisterIdentityServer(gatewayClient, identserver.NewFromProvider(ip))

	hm := health.NewAggregator(health.WithStaticAnnotations(map[string]string{
		annotations.AgentVersion: annotations.Version2,
	}))
	controlv1.RegisterHealthServer(gatewayClient, hm)

	pl.Hook(hooks.OnLoadMC(func(hc controlv1.HealthClient, m meta.PluginMeta, cc *grpc.ClientConn) {
		client := controlv1.NewHealthClient(cc)
		hm.AddClient(m.Filename(), client)
	}))

	pl.Hook(hooks.OnLoadMC(func(ext types.StreamAPIExtensionPlugin, md meta.PluginMeta, cc *grpc.ClientConn) {
		lg.With(
			zap.String("plugin", md.Module),
		).Debug("loaded stream api extension plugin")
		gatewayClient.RegisterSplicedStream(cc, md.Filename())
	}))

	return &Agent{
		AgentOptions: options,
		config:       conf.Spec,
		router:       router,
		Logger:       lg,
		pluginLoader: pl,

		tenantID:         id,
		identityProvider: ip,
		keyringStore:     ks,
		trust:            trust,
		gatewayClient:    gatewayClient,
		pluginSyncer:     pluginUpgrader,
		agentSyncer:      upgrader,

		healthzMu: healthzMu,
		healthz:   healthz,

		loadedExistingKeyring: loadedExistingKeyring,
	}, nil
}

func (a *Agent) ListenAndServe(ctx context.Context) error {
	syncClient := controlv1.NewUpdateSyncClient(a.gatewayClient.ClientConn())

	agentSyncConf := update.SyncConfig{
		Client: syncClient,
		Syncer: a.agentSyncer,
		Logger: a.Logger.Named("agent-updater"),
	}
	pluginSyncConf := update.SyncConfig{
		Client:      syncClient,
		StatsClient: a.gatewayClient,
		Syncer:      a.pluginSyncer,
		Logger:      a.Logger.Named("plugin-updater"),
	}

	for _, conf := range []update.SyncConfig{agentSyncConf, pluginSyncConf} {
		for ctx.Err() == nil {
			err := conf.DoSync(ctx)
			if err != nil {
				switch status.Code(err) {
				case codes.Unauthenticated:
					if a.loadedExistingKeyring {
						a.Logger.With(
							zap.Error(err),
						).Warn("The agent failed to authorize to the gateway using an existing keyring. " +
							"This could be due to a leftover keyring from a previous installation that was not deleted.",
						)
						if a.config.ContainsBootstrapCredentials() {
							a.Logger.Warn("Bootstrap credentials have been provided in the config file - " +
								"the agent will restart and attempt to re-bootstrap a new keyring using these credentials.")
							return ErrRebootstrap
						}
					}
				case codes.Unavailable:
					a.Logger.With(
						zap.Error(err),
					).Warn("error syncing manifest (retrying)")
					continue
				}
				return fmt.Errorf("error syncing manifest: %w", err)
			}
			break
		}
	}
	agentManifest, err := agentSyncConf.Result(ctx)
	if err != nil {
		return fmt.Errorf("error getting updated agent manifest: %w", err)
	}

	pluginManifest, err := pluginSyncConf.Result(ctx)
	if err != nil {
		return fmt.Errorf("error getting updated plugin manifest: %w", err)
	}

	if a.unmanagedPluginLoader == nil {
		done := make(chan struct{})
		a.pluginLoader.Hook(hooks.OnLoadingCompleted(func(numPlugins int) {
			a.Logger.Infof("loaded %d plugins", numPlugins)
			close(done)
		}))

		a.pluginLoader.LoadPlugins(ctx, a.config.PluginDir, plugins.AgentScheme,
			plugins.WithManifest(pluginManifest),
		)

		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	} else {
		a.Logger.Info("using unmanaged plugin loader")
	}
	// eventually passed to runGatewayClient
	buildInfo, ok := versions.ReadBuildInfo()
	if !ok {
		return fmt.Errorf("error reading build info")
	}

	buildInfoData, err := protojson.Marshal(buildInfo)
	if err != nil {
		return err
	}
	ctx = metadata.AppendToOutgoingContext(ctx,
		controlv1.AgentBuildInfoKey, string(buildInfoData),
		controlv1.ManifestDigestKeyForType(urn.Agent), agentManifest.Digest(),
		controlv1.ManifestDigestKeyForType(urn.Plugin), pluginManifest.Digest(),
		controlv1.UpdateStrategyKeyForType(urn.Agent), a.agentSyncer.Strategy(),
		controlv1.UpdateStrategyKeyForType(urn.Plugin), a.pluginSyncer.Strategy(),
	)

	listener, err := net.Listen("tcp4", a.config.ListenAddress)
	if err != nil {
		return err
	}
	a.Logger.With(
		zap.String("address", listener.Addr().String()),
	).Info("agent http server starting")

	ctx, ca := context.WithCancel(ctx)

	e1 := lo.Async(func() error {
		return util.ServeHandler(ctx, a.router.Handler(), listener)
	})

	e2 := lo.Async(func() error {
		return a.runGatewayClient(ctx)
	})

	return util.WaitAll(ctx, ca, e1, e2)
}

func (a *Agent) ListenAddress() string {
	return a.config.ListenAddress
}

func setupPluginRoutes(
	lg *zap.SugaredLogger,
	mutex *sync.Mutex,
	router *gin.Engine,
	cfg *apiextensions.HTTPAPIExtensionConfig,
	pluginMeta meta.PluginMeta,
	reservedPrefixRoutes []string,
) {
	mutex.Lock()
	defer mutex.Unlock()

	sampledLogger := logger.New(
		logger.WithSampling(&slogsampling.ThresholdSamplingOption{
			Threshold: 1,
			Rate:      0,
		}),
	).WithGroup("api")
	forwarder := fwd.To(cfg.HttpAddr,
		fwd.WithLogger(sampledLogger),
		fwd.WithDestHint(pluginMeta.Filename()),
	)
ROUTES:
	for _, route := range cfg.Routes {
		for _, reservedPrefix := range reservedPrefixRoutes {
			if strings.HasPrefix(route.Path, reservedPrefix) {
				lg.With(
					"route", route.Method+" "+route.Path,
					"plugin", pluginMeta.Module,
				).Warn("skipping route for plugin as it conflicts with a reserved prefix")
				continue ROUTES
			}
		}
		lg.With(
			"route", route.Method+" "+route.Path,
			"plugin", pluginMeta.Module,
		).Debug("configured route for plugin")
		router.Handle(route.Method, route.Path, forwarder)
	}
}

func (a *Agent) runGatewayClient(ctx context.Context) error {
	lg := a.Logger
	isRetry := false
	for ctx.Err() == nil {
		if isRetry {
			time.Sleep(1 * time.Second)
			lg.Info("attempting to reconnect...")
		} else {
			lg.Info("connecting to gateway...")
		}
		// this connects plugin extension servers to the agent's totem server.
		// clients on the other side of this stream will have access to gateway
		// services.
		_, errF := a.gatewayClient.Connect(ctx) // this unused cc can access all services
		if !errF.IsSet() {
			if isRetry {
				lg.Info("gateway reconnected")
			} else {
				lg.Info("gateway connected")
			}

			a.healthzMu.Lock()
			*a.healthz &^= healthzGatewayNotConnected
			a.healthzMu.Unlock()

			err := errF.Get() // this will block until an error is received
			if status.Code(err) == codes.Canceled {
				lg.Info("gateway client stopped")
			} else {
				lg.With(zap.Error(errF.Get())).Warn("disconnected from gateway")
			}

			a.healthzMu.Lock()
			*a.healthz |= healthzGatewayNotConnected
			a.healthzMu.Unlock()
		} else {
			lg.With(
				zap.Error(errF.Get()),
			).Warn("error connecting to gateway")
		}

		switch util.StatusCode(errF.Get()) {
		case codes.FailedPrecondition:
			// this error will be returned if the agent needs to restart
			lg.Warn("encountered non-retriable error")
			return errF.Get()
		case codes.Unauthenticated:
			return errF.Get()
		}
		isRetry = true
	}
	return ctx.Err()
}
