package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/kralicky/opni-monitoring/pkg/b2bmac"
	"github.com/kralicky/opni-monitoring/pkg/bootstrap"
	"github.com/kralicky/opni-monitoring/pkg/config/v1beta1"
	"github.com/kralicky/opni-monitoring/pkg/ident"
	"github.com/kralicky/opni-monitoring/pkg/keyring"
	"github.com/kralicky/opni-monitoring/pkg/storage"
	"github.com/valyala/fasthttp"
)

type Agent struct {
	AgentOptions
	v1beta1.AgentConfigSpec
	app *fiber.App

	tenantID  string
	tlsConfig *tls.Config

	identityProvider ident.Provider
	keyringStore     storage.KeyringStore

	sharedKeys *keyring.SharedKeys
	tlsKey     *keyring.TLSKey
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

func New(conf *v1beta1.AgentConfig, opts ...AgentOption) *Agent {
	options := AgentOptions{}
	options.Apply(opts...)

	if options.bootstrapper == nil {
		panic("bootstrapper is required")
	}

	app := fiber.New(fiber.Config{
		Prefork:           false,
		StrictRouting:     false,
		AppName:           "Opni Monitoring Agent",
		ReduceMemoryUsage: false,
		Network:           "tcp4",
	})

	app.Use(logger.New(), compress.New())
	app.All("/healthz", func(c *fiber.Ctx) error {
		return c.SendStatus(fasthttp.StatusOK)
	})

	agent := &Agent{
		AgentOptions:    options,
		AgentConfigSpec: conf.Spec,
		app:             app,
	}

	switch agent.IdentityProvider.Type {
	case v1beta1.IdentityProviderKubernetes:
		agent.identityProvider = ident.NewKubernetesProvider()
	case v1beta1.IdentityProviderHostPath:
		agent.identityProvider = ident.NewHostPathProvider(agent.IdentityProvider.Options["path"])
	default:
		log.Fatal("Unknown identity provider: ", agent.IdentityProvider.Type)
	}

	switch agent.Storage.Type {
	case v1beta1.StorageTypeEtcd:
		etcd := storage.NewEtcdStore(storage.WithClientConfig(agent.Storage.Etcd.Config))
		id, err := agent.identityProvider.UniqueIdentifier(context.Background())
		if err != nil {
			log.Fatal(err)
		}
		ks, err := etcd.KeyringStore(context.Background(), id)
		if err != nil {
			log.Fatal(err)
		}
		agent.keyringStore = ks
	case v1beta1.StorageTypeSecret:
		agent.keyringStore = storage.NewInClusterSecretStore()
	default:
		log.Fatal("Unknown storage type: ", agent.Storage.Type)
	}

	agent.bootstrapOrLoadKeys()
	agent.tlsConfig = agent.tlsKey.TLSConfig.ToCryptoTLSConfig()

	app.Post("/api/v1/push", agent.handlePushRequest)
	app.Use(default404Handler)

	return agent
}

func (a *Agent) handlePushRequest(c *fiber.Ctx) error {
	nonce, sig, err := b2bmac.New512(a.tenantID, c.Body(), a.sharedKeys.ClientKey)
	if err != nil {
		log.Fatal("Error using shared client key to generate a MAC: ", err)
	}
	authHeader, err := b2bmac.EncodeAuthHeader(a.tenantID, nonce, sig)
	if err != nil {
		log.Fatal("Error encoding auth header: ", err)
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
		log.Printf("Error sending request to gateway: %s", err)
	}

	return c.Status(code).Send(body)
}

func (a *Agent) ListenAndServe() error {
	return a.app.Listen(a.ListenAddress)
}

func (a *Agent) bootstrapOrLoadKeys() {
	// Look up our tenant ID
	id, err := a.identityProvider.UniqueIdentifier(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	a.tenantID = id

	// Load the stored keyring, or bootstrap a new one if it doesn't exist
	kr, err := a.keyringStore.Get(context.Background())
	if errors.Is(err, storage.ErrNotFound) {
		fmt.Println("Performing initial bootstrap...")
		kr, err = a.bootstrapper.Bootstrap(context.Background(), a.identityProvider)
		if err != nil {
			log.Fatal(fmt.Errorf("Bootstrap failed: %w", err))
		}
		fmt.Println("Bootstrap OK")
		for {
			// Don't let this fail easily, otherwise we will lose the keyring forever.
			// Keep retrying until it succeeds.
			err = a.keyringStore.Put(context.Background(), kr)
			if err != nil {
				fmt.Fprintln(os.Stderr, fmt.Errorf("Failed to persist keyring (retry in 1 second): %w", err))
				time.Sleep(1 * time.Second)
			} else {
				break
			}
		}
	} else if err != nil {
		log.Fatal(fmt.Errorf("Failed to load keyring: %w", err))
	}

	// Get keys from the keyring
	kr.Try(
		func(shared *keyring.SharedKeys) {
			a.sharedKeys = shared
		},
		func(tls *keyring.TLSKey) {
			a.tlsKey = tls
		},
	)
	if a.sharedKeys == nil || a.tlsKey == nil {
		log.Fatal("keyring does not contain the expected keys")
	}
}
