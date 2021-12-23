package gateway

import (
	"crypto/tls"
	"crypto/x509"
	"log"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/middleware/proxy"
	"github.com/kralicky/opni-gateway/pkg/config"
)

type Gateway struct {
	GatewayOptions
	config *config.GatewayConfig
	app    *fiber.App
}

func default404Handler(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNotFound)
}

func NewGateway(gc *config.GatewayConfig, opts ...GatewayOption) *Gateway {
	options := GatewayOptions{
		fiberMiddlewares: []FiberMiddleware{},
	}
	options.Apply(opts...)

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

	g := &Gateway{
		GatewayOptions: options,
		config:         gc,
		app:            app,
	}
	g.setupQueryRoutes(app)
	g.setupPushRoutes(app)
	g.setupBootstrapEndpoint(app)

	app.Use(default404Handler)

	return g
}

func (g *Gateway) Listen() error {
	if g.keypair == nil {
		return g.app.Listen(g.listenAddr)
	}

	pool := x509.NewCertPool()
	pool.AddCert(g.rootCA)

	config := &tls.Config{
		Certificates:     []tls.Certificate{*g.keypair},
		ClientAuth:       tls.VerifyClientCertIfGiven,
		CurvePreferences: []tls.CurveID{tls.X25519},
		RootCAs:          pool,
	}
	listener, err := tls.Listen(g.app.Config().Network, g.listenAddr, config)
	if err != nil {
		return err
	}
	return g.app.Listener(listener)
}

func (g *Gateway) Shutdown() error {
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
