package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rancher/opni-monitoring/pkg/bootstrap"
	"github.com/rancher/opni-monitoring/pkg/clients"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/ident"
	"github.com/rancher/opni-monitoring/pkg/keyring"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/storage/etcd"
	"github.com/rancher/opni-monitoring/pkg/storage/secrets"
	"github.com/rancher/opni-monitoring/pkg/util"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type Agent struct {
	AgentOptions
	v1beta1.AgentConfigSpec
	app    *fiber.App
	logger *zap.SugaredLogger

	tenantID         string
	identityProvider ident.Provider
	keyringStore     storage.KeyringStore
	gatewayClient    clients.GatewayHTTPClient
}

type AgentOptions struct {
	bootstrapper bootstrap.Bootstrapper
}

type AgentOption func(*AgentOptions)

func (o *AgentOptions) Apply(opts ...AgentOption) {
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
	options.Apply(opts...)

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

	switch agent.Storage.Type {
	case v1beta1.StorageTypeEtcd:
		etcdStore := etcd.NewEtcdStore(ctx, agent.Storage.Etcd)
		agent.keyringStore = util.Must(etcdStore.KeyringStore(ctx, "agent", &core.Reference{
			Id: id,
		}))
	case v1beta1.StorageTypeSecret:
		secStore := secrets.NewSecretsStore()
		agent.keyringStore = util.Must(secStore.KeyringStore(ctx, "agent", &core.Reference{
			Id: id,
		}))
	default:
		return nil, errors.New("unknown storage type")
	}

	var kr keyring.Keyring
	if options.bootstrapper != nil {
		if kr, err = agent.bootstrap(ctx); err != nil {
			return nil, fmt.Errorf("bootstrap error: %w", err)
		}
	} else {
		if kr, err = agent.loadKeyring(ctx); err != nil {
			return nil, fmt.Errorf("error loading keyring: %w", err)
		}
	}

	agent.gatewayClient, err = clients.NewGatewayHTTPClient(
		conf.Spec.GatewayAddress, ip, kr)
	if err != nil {
		return nil, fmt.Errorf("error configuring gateway client: %w", err)
	}
	go agent.streamRulesToGateway(ctx)

	app.Post("/api/agent/push", agent.handlePushRequest)
	app.Use(default404Handler)

	return agent, nil
}

func (a *Agent) handlePushRequest(c *fiber.Ctx) error {
	code, body, err := a.gatewayClient.Post(context.Background(), "/api/agent/push").
		Body(c.Body()).
		Send()
	if err != nil {
		return err
	}
	return c.Status(code).Send(body)
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
			return nil, fmt.Errorf("bootstrap failed: %w", err)
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
		lg.With(zap.Error(err)).Error("error in post-bootstrap finalization")
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
