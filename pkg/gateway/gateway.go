package gateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/middleware/proxy"
	tenantauth "github.com/kralicky/opni-gateway/pkg/auth/tenant"
	"github.com/kralicky/opni-gateway/pkg/bootstrap"
	"github.com/kralicky/opni-gateway/pkg/config"
	"github.com/kralicky/opni-gateway/pkg/config/v1beta1"
	"github.com/kralicky/opni-gateway/pkg/management"
	"github.com/kralicky/opni-gateway/pkg/storage"
	"github.com/kralicky/opni-gateway/pkg/util"
)

type Gateway struct {
	GatewayOptions
	config              *config.GatewayConfig
	app                 *fiber.App
	managementServer    *management.Server
	managementCtx       context.Context
	managementCtxCancel context.CancelFunc
	tlsConfig           *tls.Config
	servingCertBundle   *tls.Certificate
}

func default404Handler(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNotFound)
}

func NewGateway(conf *config.GatewayConfig, opts ...GatewayOption) *Gateway {
	options := GatewayOptions{
		fiberMiddlewares: []FiberMiddleware{},
	}
	options.Apply(opts...)

	var tokenStore storage.TokenStore
	var tenantStore storage.TenantStore
	switch conf.Spec.Storage.Type {
	case v1beta1.StorageTypeEtcd:
		options := conf.Spec.Storage.Etcd
		if options == nil {
			log.Fatal("etcd storage options missing from config")
		}
		store := storage.NewEtcdStore(storage.WithClientConfig(options.Config))
		tokenStore = store
		tenantStore = store
	default:
		log.Fatalf("unknown storage type %q", conf.Spec.Storage.Type)
	}

	conf.Spec.SetDefaults()

	if options.authMiddleware == nil {
		log.Fatal("auth middleware is required")
	}

	app := fiber.New(fiber.Config{
		Prefork:                 options.prefork,
		StrictRouting:           false,
		AppName:                 "Opni Gateway",
		ReduceMemoryUsage:       false,
		Network:                 "tcp4",
		EnableTrustedProxyCheck: len(conf.Spec.TrustedProxies) > 0,
		TrustedProxies:          conf.Spec.TrustedProxies,
	})

	for _, middleware := range options.fiberMiddlewares {
		app.Use(middleware)
	}

	if conf.Spec.EnableMonitor {
		app.Get("/monitor", monitor.New())
	}

	app.All("/healthz", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).SendString("alive")
	})

	servingCertBundle, err := loadCerts(
		conf.Spec.Certs.CACert,
		conf.Spec.Certs.ServingCert,
		conf.Spec.Certs.ServingKey,
	)
	if err != nil {
		log.Fatal(err)
	}

	pool := x509.NewCertPool()
	rootCA, err := x509.ParseCertificate(
		servingCertBundle.Certificate[len(servingCertBundle.Certificate)-1])
	if err != nil {
		log.Fatal(err)
	}
	if !rootCA.IsCA || rootCA.Subject.String() != rootCA.Issuer.String() {
		log.Fatal("No root CA given in certificate chain")
	}
	pool.AddCert(rootCA)

	tlsConfig := &tls.Config{
		Certificates:     []tls.Certificate{*servingCertBundle},
		CurvePreferences: []tls.CurveID{tls.X25519},
		RootCAs:          pool,
	}

	mgmtSrv := management.NewServer(
		management.TokenStore(tokenStore),
		management.TenantStore(tenantStore),
		management.Socket(conf.Spec.ManagementSocket),
		management.TLSConfig(tlsConfig),
	)

	g := &Gateway{
		GatewayOptions:    options,
		config:            conf,
		app:               app,
		managementServer:  mgmtSrv,
		tlsConfig:         tlsConfig,
		servingCertBundle: servingCertBundle,
	}
	g.setupQueryRoutes(app)
	g.setupPushRoutes(app, tenantStore)

	app.Post("/bootstrap/*", bootstrap.ServerConfig{
		RootCA:      rootCA,
		Certificate: servingCertBundle,
		TokenStore:  tokenStore,
		TenantStore: tenantStore,
	}.Handle).Use(limiter.New()) // Limit requests to 5 per minute

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

	if g.servingCertBundle == nil {
		return g.app.Listen(g.config.Spec.ListenAddress)
	}

	listener, err := tls.Listen(g.app.Config().Network,
		g.config.Spec.ListenAddress, g.tlsConfig)
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

func (g *Gateway) setupPushRoutes(app *fiber.App, tenantStore storage.TenantStore) {
	distributor := proxy.Balancer(proxy.Config{
		Servers: []string{g.config.Spec.Cortex.Distributor.Address},
	})
	tenantMiddleware := tenantauth.New(tenantStore)
	v1 := app.Group("/api/v1", tenantMiddleware.Handle)
	v1.Post("/push", distributor)
}

// Returns a complete cert chain including the root CA, and a tls serving cert.
func loadCerts(cacertPath, certPath, keyPath string) (*tls.Certificate, error) {
	data, err := os.ReadFile(cacertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load CA cert: %v", err)
	}
	root, err := util.ParsePEMEncodedCertChain(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA cert: %v", err)
	}
	if len(root) != 1 {
		return nil, fmt.Errorf("failed to parse CA cert: expected one certificate in chain, got %d", len(root))
	}
	rootCA := root[0]
	servingCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS certificate: %w", err)
	}
	servingRootData := servingCert.Certificate[len(servingCert.Certificate)-1]
	servingRoot, err := x509.ParseCertificate(servingRootData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse serving root certificate: %w", err)
	}
	if !rootCA.Equal(servingRoot) {
		servingCert.Certificate = append(servingCert.Certificate, rootCA.Raw)
	}
	return &servingCert, nil
}
