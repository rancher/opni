package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	gsync "github.com/kralicky/gpkg/sync"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
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
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type conditionStatus int32

const (
	statusPending conditionStatus = 0
	statusFailure conditionStatus = 1
)

func (s conditionStatus) String() string {
	switch s {
	case statusPending:
		return "Pending"
	case statusFailure:
		return "Failure"
	}
	return ""
}

const (
	condRemoteWrite = "Remote Write"
	condRuleSync    = "Rule Sync"
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
	trust            trust.Strategy

	remoteWriteClient clients.Locker[remotewrite.RemoteWriteClient]

	conditions gsync.Map[string, conditionStatus]
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

	initCtx, initCancel := context.WithTimeout(ctx, 10*time.Second)
	defer initCancel()

	ip, err := ident.GetProvider(conf.Spec.IdentityProvider)
	if err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}
	id, err := ip.UniqueIdentifier(initCtx)
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
	agent.initConditions()

	var keyringStoreBroker storage.KeyringStoreBroker
	switch agent.Storage.Type {
	case v1beta1.StorageTypeEtcd:
		keyringStoreBroker = etcd.NewEtcdStore(ctx, agent.Storage.Etcd)
	case v1beta1.StorageTypeCRDs:
		keyringStoreBroker = crds.NewCRDStore()
	default:
		return nil, fmt.Errorf("unknown storage type: %s", agent.Storage.Type)
	}
	agent.keyringStore, err = keyringStoreBroker.KeyringStore("agent", &corev1.Reference{
		Id: id,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating keyring store: %w", err)
	}

	var kr keyring.Keyring
	if options.bootstrapper != nil {
		if kr, err = agent.bootstrap(initCtx); err != nil {
			return nil, fmt.Errorf("error during bootstrap: %w", err)
		}
	} else {
		if kr, err = agent.loadKeyring(initCtx); err != nil {
			return nil, fmt.Errorf("error loading keyring: %w", err)
		}
	}

	if conf.Spec.GatewayAddress == "" {
		return nil, errors.New("gateway address not set")
	}

	agent.trust, err = agent.buildTrustStrategy(kr)
	if err != nil {
		return nil, fmt.Errorf("error building trust strategy: %w", err)
	}

	agent.gatewayClient, err = clients.NewGatewayGRPCClient(
		conf.Spec.GatewayAddress, ip, kr, agent.trust)
	if err != nil {
		return nil, fmt.Errorf("error configuring gateway client: %w", err)
	}
	controlv1.RegisterAgentControlServer(agent.gatewayClient, agent)

	var startRuleStreamOnce sync.Once
	// var startServiceDiscoveryStreamOnce sync.Once
	go func() {
		for ctx.Err() == nil {
			cc, errF := agent.gatewayClient.Connect(ctx)
			if !errF.IsSet() {
				agent.remoteWriteClient = clients.NewLocker(cc, remotewrite.NewRemoteWriteClient)

				startRuleStreamOnce.Do(func() {
					go agent.streamRulesToGateway(ctx)
				})

				// TODO : Implement
				// startServiceDiscoveryStreamOnce.Do(func() {
				// 	go agent.streamServiceDiscoveryToGateway(ctx)
				// })

				lg.Error(errF.Get())
				agent.remoteWriteClient.Close()
			} else {
				lg.Error(errF.Get())
			}
		}
	}()

	app.Post("/api/agent/push", agent.handlePushRequest)

	return agent, nil
}

func (a *Agent) handlePushRequest(c *fiber.Ctx) error {
	var status int
	ok := a.remoteWriteClient.Use(func(rwc remotewrite.RemoteWriteClient) {
		if rwc == nil {
			a.conditions.Store(condRemoteWrite, statusPending)
			status = fiber.StatusServiceUnavailable
			return
		}
		_, err := rwc.Push(c.Context(), &remotewrite.Payload{
			AuthorizedClusterID: a.tenantID,
			Contents:            c.Body(),
		})
		if err != nil {
			a.logger.Errorf("remote write error: %v", err)
			a.conditions.Store(condRemoteWrite, statusFailure)
			status = fiber.StatusServiceUnavailable
			return
		}
		a.conditions.Delete(condRemoteWrite)
		status = fiber.StatusOK
	})
	if !ok {
		status = fiber.StatusServiceUnavailable
	}
	return c.SendStatus(status)
}

func (a *Agent) ListenAndServe() error {
	return a.app.Listen(a.ListenAddress)
}

func (a *Agent) Shutdown() error {
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

func (a *Agent) GetHealth(context.Context, *emptypb.Empty) (*corev1.Health, error) {
	conditions := []string{}
	a.conditions.Range(func(key string, value conditionStatus) bool {
		conditions = append(conditions, fmt.Sprintf("%s %s", key, value))
		return true
	})

	return &corev1.Health{
		Ready:      len(conditions) == 0,
		Conditions: conditions,
	}, nil
}

func (a *Agent) initConditions() {
	a.conditions.Store(condRemoteWrite, statusPending)
	a.conditions.Store(condRuleSync, statusPending)
}

func (a *Agent) buildTrustStrategy(kr keyring.Keyring) (trust.Strategy, error) {
	var trustStrategy trust.Strategy
	var err error
	switch a.TrustStrategy {
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
		return nil, fmt.Errorf("unknown trust strategy: %s", a.TrustStrategy)
	}

	return trustStrategy, nil
}

func (a *Agent) GetKeyring(ctx context.Context) (keyring.Keyring, error) {
	return a.keyringStore.Get(ctx)
}

func (a *Agent) GetTrustStrategy() trust.Strategy {
	return a.trust
}
