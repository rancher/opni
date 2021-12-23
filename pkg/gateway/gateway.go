package gateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/middleware/proxy"
	"github.com/kralicky/opni-gateway/pkg/config"
	"github.com/kralicky/opni-gateway/pkg/management"
	"github.com/kralicky/opni-gateway/pkg/storage"
)

type Gateway struct {
	GatewayOptions
	config              *config.GatewayConfig
	app                 *fiber.App
	managementServer    *management.Server
	managementCtx       context.Context
	managementCtxCancel context.CancelFunc
	tokenStore          storage.TokenStore
}

func default404Handler(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNotFound)
}

func NewGateway(gc *config.GatewayConfig, opts ...GatewayOption) *Gateway {
	options := GatewayOptions{
		fiberMiddlewares: []FiberMiddleware{},
		managementSocket: management.DefaultManagementSocket,
	}
	options.Apply(opts...)

	var tokenStore storage.TokenStore
	switch strings.ToLower(strings.TrimSpace(gc.Spec.Storage.Type)) {
	case "etcd":
		options := gc.Spec.Storage.Etcd
		if options == nil {
			log.Fatal("etcd storage options missing from config")
		}
		ts, err := storage.NewEtcdTokenStore(storage.WithClientConfig(options.Config))
		if err != nil {
			log.Fatal(fmt.Errorf("failed to initialize etcd token store: %w", err))
		}
		tokenStore = ts
	default:
		log.Fatalf("unknown storage type %q", gc.Spec.Storage.Type)
	}

	gc.Spec.SetDefaults()

	if options.authMiddleware == nil {
		log.Fatal("auth middleware is required")
	}

	app := fiber.New(fiber.Config{
		Prefork:                 options.prefork,
		StrictRouting:           false,
		AppName:                 "Opni Gateway",
		ReduceMemoryUsage:       false,
		Network:                 "tcp4",
		EnableTrustedProxyCheck: len(options.trustedProxies) > 0,
		TrustedProxies:          options.trustedProxies,
	})

	for _, middleware := range options.fiberMiddlewares {
		app.Use(middleware)
	}

	if options.enableMonitor {
		app.Get("/monitor", monitor.New())
	}

	app.All("/healthz", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).SendString("alive")
	})

	mgmtSrv := management.NewServer(
		management.TokenStore(tokenStore),
		management.Socket(options.managementSocket),
	)

	g := &Gateway{
		GatewayOptions:   options,
		config:           gc,
		app:              app,
		managementServer: mgmtSrv,
		tokenStore:       tokenStore,
	}
	g.setupQueryRoutes(app)
	g.setupPushRoutes(app)
	g.setupBootstrapEndpoint(app)

	app.Use(default404Handler)

	return g
}

func (g *Gateway) Listen() error {
	g.managementCtx, g.managementCtxCancel =
		context.WithCancel(context.Background())
	go func() {
		if err := g.managementServer.ListenAndServe(g.managementCtx); err != nil {
			panic(err)
		}
	}()

	if g.keypair == nil {
		return g.app.Listen(g.httpListenAddr)
	}

	pool := x509.NewCertPool()
	pool.AddCert(g.rootCA)

	config := &tls.Config{
		Certificates:     []tls.Certificate{*g.keypair},
		ClientAuth:       tls.VerifyClientCertIfGiven,
		CurvePreferences: []tls.CurveID{tls.X25519},
		RootCAs:          pool,
	}
	listener, err := tls.Listen(g.app.Config().Network, g.httpListenAddr, config)
	if err != nil {
		return err
	}
	return g.app.Listener(listener)
}

func (g *Gateway) Shutdown() error {
	g.managementCtxCancel()
	return g.app.Shutdown()
}

func (g *Gateway) setupQueryRoutes(app *fiber.App) {
	queryFrontend := proxy.Balancer(proxy.Config{
		Servers: []string{g.config.Spec.Cortex.QueryFrontend.Address},
	})
	alertmanager := proxy.Balancer(proxy.Config{
		Servers: []string{g.config.Spec.Cortex.Alertmanager.Address},
	})
	ruler := proxy.Balancer(proxy.Config{
		Servers: []string{g.config.Spec.Cortex.Ruler.Address},
	})

	// Distributor
	app.Get("/services", queryFrontend)
	app.Get("/ready", queryFrontend)

	// Alertmanager UI
	alertmanagerUi := app.Group("/alertmanager", g.authMiddleware.Handle)
	alertmanagerUi.Get("/alertmanager", alertmanager)

	// Prometheus-compatible API
	promv1 := app.Group("/prometheus/api/v1", g.authMiddleware.Handle)

	// GET, POST
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		promv1.Add(method, "/query", queryFrontend)
		promv1.Add(method, "/query_range", queryFrontend)
		promv1.Add(method, "/query_exemplars", queryFrontend)
		promv1.Add(method, "/series", queryFrontend)
		promv1.Add(method, "/labels", queryFrontend)
	}
	// GET only
	promv1.Get("/rules", ruler)
	promv1.Get("/alerts", ruler)
	promv1.Get("/label/:name/values", queryFrontend)
	promv1.Get("/metadata", queryFrontend)

	// POST only
	promv1.Post("/read", queryFrontend)
}

func (g *Gateway) setupPushRoutes(app *fiber.App) {
	distributor := proxy.Balancer(proxy.Config{
		Servers: []string{g.config.Spec.Cortex.Distributor.Address},
	})

	v1 := app.Group("/api/v1", g.authMiddleware.Handle)
	v1.Post("/push", distributor)
}
