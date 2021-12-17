package gateway

import (
	"log"
	"net/http"
	"time"

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
	g.setupRoutes(app)

	app.Use(default404Handler)

	return g
}

func (g *Gateway) Listen() error {
	return g.app.Listen(g.listenAddr)
}

func (g *Gateway) Shutdown() error {
	return g.app.Shutdown()
}

func (g *Gateway) setupRoutes(app *fiber.App) {
	timeout := 10 * time.Second
	distributor := proxy.Balancer(proxy.Config{
		Servers: []string{g.config.Spec.Cortex.Distributor.Address},
		Timeout: timeout,
	})
	queryFrontend := proxy.Balancer(proxy.Config{
		Servers: []string{g.config.Spec.Cortex.QueryFrontend.Address},
		Timeout: timeout,
	})
	alertmanager := proxy.Balancer(proxy.Config{
		Servers: []string{g.config.Spec.Cortex.Alertmanager.Address},
		Timeout: timeout,
	})
	ruler := proxy.Balancer(proxy.Config{
		Servers: []string{g.config.Spec.Cortex.Ruler.Address},
		Timeout: timeout,
	})

	// Distributor
	app.Get("/services", distributor)
	app.Get("/ready", distributor)

	// Alertmanager UI
	alertmanagerUi := app.Group("/alertmanager", g.authMiddleware.Handle)
	alertmanagerUi.Get("/alertmanager", alertmanager)

	v1 := app.Group("/api/v1", g.authMiddleware.Handle)
	v1.Post("/push", distributor)

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
