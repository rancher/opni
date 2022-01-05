package proxy

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
	"github.com/kralicky/opni-gateway/pkg/b2bmac"
	"github.com/kralicky/opni-gateway/pkg/bootstrap"
	"github.com/kralicky/opni-gateway/pkg/config/v1beta1"
	"github.com/kralicky/opni-gateway/pkg/ident"
	"github.com/kralicky/opni-gateway/pkg/keyring"
	"github.com/kralicky/opni-gateway/pkg/storage"
	"github.com/valyala/fasthttp"
)

type RemoteWriteProxy struct {
	RemoteWriteProxyOptions
	v1beta1.ProxyConfigSpec
	app *fiber.App

	tenantID  string
	tlsConfig *tls.Config

	identityProvider ident.Provider
	keyringStore     storage.KeyringStore

	sharedKeys *keyring.SharedKeys
	tlsKey     *keyring.TLSKey
}

type RemoteWriteProxyOptions struct {
	bootstrapper bootstrap.Bootstrapper
}

type RemoteWriteProxyOption func(*RemoteWriteProxyOptions)

func (o *RemoteWriteProxyOptions) Apply(opts ...RemoteWriteProxyOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithBootstrapper(bootstrapper bootstrap.Bootstrapper) RemoteWriteProxyOption {
	return func(o *RemoteWriteProxyOptions) {
		o.bootstrapper = bootstrapper
	}
}

func default404Handler(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNotFound)
}

func NewRemoteWriteProxy(conf *v1beta1.ProxyConfig, opts ...RemoteWriteProxyOption) *RemoteWriteProxy {
	options := RemoteWriteProxyOptions{}
	options.Apply(opts...)

	if options.bootstrapper == nil {
		panic("bootstrapper is required")
	}

	app := fiber.New(fiber.Config{
		Prefork:           false,
		StrictRouting:     false,
		AppName:           "Opni Gateway Proxy",
		ReduceMemoryUsage: false,
		Network:           "tcp4",
	})

	app.Use(logger.New(), compress.New())
	app.All("/healthz", func(c *fiber.Ctx) error {
		return c.SendStatus(fasthttp.StatusOK)
	})

	proxy := &RemoteWriteProxy{
		RemoteWriteProxyOptions: options,
		ProxyConfigSpec:         conf.Spec,
		app:                     app,
	}

	switch proxy.IdentityProvider.Type {
	case v1beta1.IdentityProviderKubernetes:
		proxy.identityProvider = ident.NewKubernetesProvider()
	case v1beta1.IdentityProviderHostPath:
		proxy.identityProvider = ident.NewHostPathProvider(proxy.IdentityProvider.Options["path"])
	default:
		log.Fatal("Unknown identity provider: ", proxy.IdentityProvider.Type)
	}

	switch proxy.Storage.Type {
	case v1beta1.StorageTypeEtcd:
		etcd := storage.NewEtcdStore(storage.WithClientConfig(proxy.Storage.Etcd.Config))
		id, err := proxy.identityProvider.UniqueIdentifier(context.Background())
		if err != nil {
			log.Fatal(err)
		}
		ks, err := etcd.KeyringStore(context.Background(), id)
		if err != nil {
			log.Fatal(err)
		}
		proxy.keyringStore = ks
	case v1beta1.StorageTypeSecret:
		proxy.keyringStore = storage.NewInClusterSecretStore()
	default:
		log.Fatal("Unknown storage type: ", proxy.Storage.Type)
	}

	proxy.bootstrapOrLoadKeys()
	proxy.tlsConfig = proxy.tlsKey.TLSConfig.ToCryptoTLSConfig()

	app.Post("/api/v1/push", proxy.handlePushRequest)
	app.Use(default404Handler)

	return proxy
}

func (p *RemoteWriteProxy) handlePushRequest(c *fiber.Ctx) error {
	nonce, sig, err := b2bmac.New512(p.tenantID, c.Body(), p.sharedKeys.ClientKey)
	if err != nil {
		log.Fatal("Error using shared client key to generate a MAC: ", err)
	}
	authHeader, err := b2bmac.EncodeAuthHeader(p.tenantID, nonce, sig)
	if err != nil {
		log.Fatal("Error encoding auth header: ", err)
	}

	a := fiber.Post(p.GatewayAddress+"/api/v1/push").
		Set("Authorization", authHeader).
		Body(c.Body()).
		TLSConfig(p.tlsConfig)

	if err := a.Parse(); err != nil {
		panic(err)
	}

	code, body, errs := a.Bytes()
	for _, err := range errs {
		log.Printf("Error sending request to gateway: %s", err)
	}

	return c.Status(code).Send(body)
}

func (p *RemoteWriteProxy) ListenAndServe() error {
	return p.app.Listen(p.ListenAddress)
}

func (p *RemoteWriteProxy) bootstrapOrLoadKeys() {
	// Look up our tenant ID
	id, err := p.identityProvider.UniqueIdentifier(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	p.tenantID = id

	// Load the stored keyring, or bootstrap a new one if it doesn't exist
	kr, err := p.keyringStore.Get(context.Background())
	if errors.Is(err, storage.ErrNotFound) {
		fmt.Println("Performing initial bootstrap...")
		kr, err = p.bootstrapper.Bootstrap(context.Background(), p.identityProvider)
		if err != nil {
			log.Fatal(fmt.Errorf("Bootstrap failed: %w", err))
		}
		fmt.Println("Bootstrap OK")
		for {
			// Don't let this fail easily, otherwise we will lose the keyring forever.
			// Keep retrying until it succeeds.
			err = p.keyringStore.Put(context.Background(), kr)
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
			p.sharedKeys = shared
		},
		func(tls *keyring.TLSKey) {
			p.tlsKey = tls
		},
	)
	if p.sharedKeys == nil || p.tlsKey == nil {
		log.Fatal("keyring does not contain the expected keys")
	}
}
