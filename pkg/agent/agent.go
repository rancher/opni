package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kralicky/totem"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/crds"
	"github.com/rancher/opni/pkg/storage/etcd"
	"github.com/rancher/opni/pkg/trust"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/remotewrite"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type Agent struct {
	controlv1.UnsafeAgentControlServer
	AgentOptions
	v1beta1.AgentConfigSpec
	app    *fiber.App
	logger *zap.SugaredLogger

	tenantID         string
	identityProvider ident.Provider
	keyringStore     storage.KeyringStore
	gatewayClient    clients.GatewayGRPCClient
	shutdownLock     sync.Mutex

	remoteWriteMu     sync.Mutex
	remoteWriteClient remotewrite.RemoteWriteClient
}

type AgentOptions struct {
	bootstrapper bootstrap.Bootstrapper
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

func default404Handler(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNotFound)
}

func New(ctx context.Context, conf *v1beta1.AgentConfig, opts ...AgentOption) (*Agent, error) {
	lg := logger.New().Named("agent")
	options := AgentOptions{}
	options.apply(opts...)

	app := fiber.New(fiber.Config{
		Prefork:               false,
		StrictRouting:         false,
		AppName:               "Opni Monitoring Agent",
		ReduceMemoryUsage:     false,
		Network:               "tcp4",
		DisableStartupMessage: true,
	})
	logger.ConfigureAppLogger(app, "agent")

	app.All("/healthz", func(c *fiber.Ctx) error {
		return c.SendStatus(fasthttp.StatusOK)
	})

	ip, err := ident.GetProvider(conf.Spec.IdentityProvider)
	if err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}
	id, err := ip.UniqueIdentifier(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting unique identifier: %w", err)
	}
	agent := &Agent{
		AgentOptions:     options,
		AgentConfigSpec:  conf.Spec,
		app:              app,
		logger:           lg,
		tenantID:         id,
		identityProvider: ip,
	}
	agent.shutdownLock.Lock()

	var keyringStoreBroker storage.KeyringStoreBroker
	switch agent.Storage.Type {
	case v1beta1.StorageTypeEtcd:
		keyringStoreBroker = etcd.NewEtcdStore(ctx, agent.Storage.Etcd)
	case v1beta1.StorageTypeCRDs:
		keyringStoreBroker = crds.NewCRDStore()
	default:
		return nil, fmt.Errorf("unknown storage type: %s", agent.Storage.Type)
	}
	agent.keyringStore, err = keyringStoreBroker.KeyringStore(ctx, "agent", &corev1.Reference{
		Id: id,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating keyring store: %w", err)
	}

	var kr keyring.Keyring
	if options.bootstrapper != nil {
		if kr, err = agent.bootstrap(ctx); err != nil {
			return nil, err
		}
	} else {
		if kr, err = agent.loadKeyring(ctx); err != nil {
			return nil, fmt.Errorf("error loading keyring: %w", err)
		}
	}

	if conf.Spec.GatewayAddress == "" {
		return nil, errors.New("gateway address not set")
	}

	var trustStrategy trust.Strategy
	switch conf.Spec.TrustStrategy {
	case v1beta1.TrustStrategyPKP:
		conf := trust.StrategyConfig{
			PKP: &trust.PKPConfig{
				Pins: trust.NewKeyringPinSource(kr),
			},
		}
		trustStrategy, err = conf.Build()
		if err != nil {
			return nil, fmt.Errorf("error configuring pkp trust from keyring: %w", err)
		}
	case v1beta1.TrustStrategyCACerts:
		conf := trust.StrategyConfig{
			CACerts: &trust.CACertsConfig{
				CACerts: trust.NewKeyringCACertsSource(kr),
			},
		}
		trustStrategy, err = conf.Build()
		if err != nil {
			return nil, fmt.Errorf("error configuring ca certs trust from keyring: %w", err)
		}
	case v1beta1.TrustStrategyInsecure:
		conf := trust.StrategyConfig{
			Insecure: &trust.InsecureConfig{},
		}
		trustStrategy, err = conf.Build()
		if err != nil {
			return nil, fmt.Errorf("error configuring insecure trust: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown trust strategy: %s", conf.Spec.TrustStrategy)
	}

	agent.gatewayClient, err = clients.NewGatewayGRPCClient(
		conf.Spec.GatewayAddress, ip, kr, trustStrategy)
	if err != nil {
		return nil, fmt.Errorf("error configuring gateway client: %w", err)
	}

	var startRuleStreamOnce sync.Once

	go func() {
		for {
			cc, err := agent.gatewayClient.Dial(ctx)
			if err != nil {
				lg.With(
					zap.Error(err),
				).Error("error dialing gateway")
				time.Sleep(time.Second)
				continue
			}
			streamClient := streamv1.NewStreamClient(cc)
			lg.Info("establishing streaming connection with gateway")
			stream, err := streamClient.Connect(ctx)
			if err != nil {
				lg.With(
					zap.Error(err),
				).Error("error connecting to gateway")
				time.Sleep(time.Second)
				continue
			}
			lg.Info("stream connected")
			defer lg.Info("stream disconnected")

			ts := totem.NewServer(stream)
			controlv1.RegisterAgentControlServer(ts, agent)

			tc, errC := ts.Serve()
			agent.remoteWriteMu.Lock()
			agent.remoteWriteClient = remotewrite.NewRemoteWriteClient(tc)
			agent.remoteWriteMu.Unlock()

			startRuleStreamOnce.Do(func() {
				go agent.streamRulesToGateway(ctx)
			})

			select {
			case <-ctx.Done():
				return
			case err := <-errC:
				lg.With(
					zap.Error(err),
				).Error("stream server error")
			}

			agent.remoteWriteMu.Lock()
			agent.remoteWriteClient = nil
			agent.remoteWriteMu.Unlock()
		}
	}()

	app.Post("/api/agent/push", agent.handlePushRequest)
	app.Use(default404Handler)

	return agent, nil
}

func (a *Agent) handlePushRequest(c *fiber.Ctx) error {
	a.remoteWriteMu.Lock()
	defer a.remoteWriteMu.Unlock()
	if a.remoteWriteClient == nil {
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	_, err := a.remoteWriteClient.Push(c.Context(), &remotewrite.Payload{
		AuthorizedClusterID: a.tenantID,
		Contents:            c.Body(),
	})
	if err != nil {
		a.logger.Error(err)
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	return c.SendStatus(fiber.StatusOK)
}

func (a *Agent) ListenAndServe() error {
	a.shutdownLock.Unlock()
	return a.app.Listen(a.ListenAddress)
}

func (a *Agent) Shutdown() error {
	a.shutdownLock.Lock()
	return a.app.Shutdown()
}

func (a *Agent) bootstrap(ctx context.Context) (keyring.Keyring, error) {
	lg := a.logger

	// Load the stored keyring, or bootstrap a new one if it doesn't exist
	if _, err := a.keyringStore.Get(ctx); errors.Is(err, storage.ErrNotFound) {
		lg.Info("performing initial bootstrap")
		newKeyring, err := a.bootstrapper.Bootstrap(ctx, a.identityProvider)
		if err != nil {
			return nil, err
		}
		lg.Info("bootstrap completed successfully")
		for {
			// Don't let this fail easily, otherwise we will lose the keyring forever.
			// Keep retrying until it succeeds.
			err = a.keyringStore.Put(ctx, newKeyring)
			if err != nil {
				lg.With(zap.Error(err)).Error("failed to persist keyring (retry in 1 second)")
				time.Sleep(1 * time.Second)
			} else {
				break
			}
		}
	} else if err != nil {
		return nil, fmt.Errorf("error loading keyring: %w", err)
	} else {
		lg.Warn("this agent has already been bootstrapped but may have been interrupted - will use existing keyring")
	}

	lg.Info("running post-bootstrap finalization steps")
	if err := a.bootstrapper.Finalize(ctx); err != nil {
		lg.With(zap.Error(err)).Warn("error in post-bootstrap finalization")
	} else {
		lg.Info("bootstrap completed successfully")
	}
	return a.loadKeyring(ctx)
}

func (a *Agent) loadKeyring(ctx context.Context) (keyring.Keyring, error) {
	lg := a.logger
	lg.Info("loading keyring")
	kr, err := a.keyringStore.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("error loading keyring: %w", err)
	}
	lg.Info("keyring loaded successfully")
	return kr, nil
}

var startTime = time.Now()

func (a *Agent) GetHealth(context.Context, *emptypb.Empty) (*corev1.Health, error) {
	return &corev1.Health{
		Ready:  true,
		Uptime: durationpb.New(time.Since(startTime)),
	}, nil
}
