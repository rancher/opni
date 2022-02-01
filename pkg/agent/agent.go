package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kralicky/opni-monitoring/pkg/b2bmac"
	"github.com/kralicky/opni-monitoring/pkg/bootstrap"
	"github.com/kralicky/opni-monitoring/pkg/config/v1beta1"
	"github.com/kralicky/opni-monitoring/pkg/core"
	"github.com/kralicky/opni-monitoring/pkg/ident"
	"github.com/kralicky/opni-monitoring/pkg/keyring"
	"github.com/kralicky/opni-monitoring/pkg/logger"
	"github.com/kralicky/opni-monitoring/pkg/pkp"
	"github.com/kralicky/opni-monitoring/pkg/storage"
	"github.com/kralicky/opni-monitoring/pkg/storage/etcd"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
)

type Agent struct {
	AgentOptions
	v1beta1.AgentConfigSpec
	app    *fiber.App
	logger *zap.SugaredLogger

	tenantID  string
	tlsConfig *tls.Config

	identityProvider ident.Provider
	keyringStore     storage.KeyringStore

	sharedKeys *keyring.SharedKeys
	pkpKey     *keyring.PKPKey
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

func New(conf *v1beta1.AgentConfig, opts ...AgentOption) (*Agent, error) {
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
	logger.ConfigureApp(app, lg)

	app.All("/healthz", func(c *fiber.Ctx) error {
		return c.SendStatus(fasthttp.StatusOK)
	})

	agent := &Agent{
		AgentOptions:    options,
		AgentConfigSpec: conf.Spec,
		app:             app,
		logger:          lg,
	}

	var err error
	agent.identityProvider, err = ident.GetProvider(conf.Spec.IdentityProvider)
	if err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}

	switch agent.Storage.Type {
	case v1beta1.StorageTypeEtcd:
		es := etcd.NewEtcdStore(agent.Storage.Etcd,
			etcd.WithNamespace("agent"),
		)
		id, err := agent.identityProvider.UniqueIdentifier(context.Background())
		if err != nil {
			return nil, fmt.Errorf("error getting unique identifier: %w", err)
		}
		ks, err := es.KeyringStore(context.Background(), &core.Reference{
			Id: id,
		})
		if err != nil {
			return nil, fmt.Errorf("error getting keyring store: %w", err)
		}
		agent.keyringStore = ks
	case v1beta1.StorageTypeSecret:
		agent.keyringStore = storage.NewInClusterSecretStore()
	default:
		return nil, errors.New("unknown storage type")
	}

	if options.bootstrapper != nil {
		if err := agent.bootstrap(); err != nil {
			return nil, fmt.Errorf("bootsrap error: %w", err)
		}
	} else {
		if err := agent.loadKeyring(); err != nil {
			return nil, fmt.Errorf("error loading keyring: %w", err)
		}
	}

	agent.tlsConfig, err = pkp.TLSConfig(agent.pkpKey.PinnedKeys)
	if err != nil {
		return nil, fmt.Errorf("error creating TLS config: %w", err)
	}

	app.Post("/api/v1/push", agent.handlePushRequest)
	app.Use(default404Handler)

	return agent, nil
}

func (a *Agent) handlePushRequest(c *fiber.Ctx) error {
	lg := a.logger
	nonce, sig, err := b2bmac.New512(a.tenantID, c.Body(), a.sharedKeys.ClientKey)
	if err != nil {
		lg.With(zap.Error(err)).Fatal("error generating MAC")
	}
	authHeader, err := b2bmac.EncodeAuthHeader(a.tenantID, nonce, sig)
	if err != nil {
		lg.With(zap.Error(err)).Fatal("error encoding auth header")
	}

	p := fiber.Post(a.GatewayAddress+"/api/v1/push").
		Set("Authorization", authHeader).
		Body(c.Body()).
		TLSConfig(a.tlsConfig)

	if err := p.Parse(); err != nil {
		panic(err)
	}

	code, body, errs := p.Bytes()
	for _, err := range errs {
		lg.With(zap.Error(err)).Error("error sending reqest to gateway")
	}

	return c.Status(code).Send(body)
}

func (a *Agent) ListenAndServe() error {
	return a.app.Listen(a.ListenAddress)
}

func (a *Agent) Shutdown() error {
	return a.app.Shutdown()
}

func (a *Agent) bootstrap() error {
	lg := a.logger
	// Look up our tenant ID
	id, err := a.identityProvider.UniqueIdentifier(context.Background())
	if err != nil {
		return fmt.Errorf("error getting unique identifier: %w", err)
	}
	a.tenantID = id

	// Load the stored keyring, or bootstrap a new one if it doesn't exist
	if _, err := a.keyringStore.Get(context.Background()); errors.Is(err, storage.ErrNotFound) {
		lg.Info("performing initial bootstrap")
		newKeyring, err := a.bootstrapper.Bootstrap(context.Background(), a.identityProvider)
		if err != nil {
			return fmt.Errorf("bootstrap failed: %w", err)
		}
		lg.Info("bootstrap completed successfully")
		for {
			// Don't let this fail easily, otherwise we will lose the keyring forever.
			// Keep retrying until it succeeds.
			err = a.keyringStore.Put(context.Background(), newKeyring)
			if err != nil {
				lg.With(zap.Error(err)).Error("failed to persist keyring (retry in 1 second)")
				time.Sleep(1 * time.Second)
			} else {
				break
			}
		}
	} else if err != nil {
		return fmt.Errorf("error loading keyring: %w", err)
	} else {
		lg.Warn("this agent has already been bootstrapped but may have been interrupted - will use existing keyring")
	}

	lg.Info("running post-bootstrap finalization steps")
	if err := a.bootstrapper.Finalize(context.Background()); err != nil {
		lg.With(zap.Error(err)).Error("error in post-bootstrap finalization")
	}
	lg.Info("bootstrap completed successfully")
	return a.loadKeyring()
}

func (a *Agent) loadKeyring() error {
	lg := a.logger
	lg.Info("loading keyring")
	kr, err := a.keyringStore.Get(context.Background())
	if err != nil {
		return fmt.Errorf("error loading keyring: %w", err)
	}
	kr.Try(
		func(shared *keyring.SharedKeys) {
			a.sharedKeys = shared
		},
		func(pkp *keyring.PKPKey) {
			a.pkpKey = pkp
		},
	)
	if a.sharedKeys == nil || a.pkpKey == nil {
		return errors.New("keyring is missing keys")
	}
	if len(a.pkpKey.PinnedKeys) == 0 {
		return errors.New("keyring does not contain any pinned public keys")
	}
	lg.Info("keyring loaded successfully")
	return nil
}
