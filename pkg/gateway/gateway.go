package gateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/utils"
	tenantauth "github.com/kralicky/opni-gateway/pkg/auth/tenant"
	"github.com/kralicky/opni-gateway/pkg/bootstrap"
	"github.com/kralicky/opni-gateway/pkg/config"
	"github.com/kralicky/opni-gateway/pkg/config/v1beta1"
	"github.com/kralicky/opni-gateway/pkg/management"
	"github.com/kralicky/opni-gateway/pkg/rbac"
	"github.com/kralicky/opni-gateway/pkg/storage"
	"github.com/kralicky/opni-gateway/pkg/util"
	"github.com/valyala/fasthttp"
)

type Gateway struct {
	GatewayOptions
	config              *config.GatewayConfig
	app                 *fiber.App
	managementServer    *management.Server
	managementCtx       context.Context
	managementCtxCancel context.CancelFunc
	tlsConfig           *tls.Config
	cortexTLSConfig     *tls.Config
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
	var rbacStore storage.RBACStore
	switch conf.Spec.Storage.Type {
	case v1beta1.StorageTypeEtcd:
		options := conf.Spec.Storage.Etcd
		if options == nil {
			log.Fatal("etcd storage options missing from config")
		}
		store := storage.NewEtcdStore(storage.WithClientConfig(options.Config))
		tokenStore = store
		tenantStore = store
		rbacStore = store
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
	if !rootCA.IsCA {
		log.Fatal("No root CA given in certificate chain")
	}
	if len(strings.TrimSpace(rootCA.Issuer.String())) == 0 ||
		len(strings.TrimSpace(rootCA.Subject.String())) == 0 {
		log.Fatal("Root CA is missing Subject or Issuer information")
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
		management.RBACStore(rbacStore),
		management.ListenAddress(conf.Spec.ManagementListenAddress),
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
	g.loadCortexCerts()
	g.setupCortexRoutes(app, tenantStore, rbacStore)

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
			log.Fatal(err)
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

func (g *Gateway) newCortexForwarder(addr string) func(*fiber.Ctx) error {
	if addr == "" {
		panic("newCortexForwarder: address is empty")
	}
	hostClient := &fasthttp.HostClient{
		NoDefaultUserAgentHeader: true,
		DisablePathNormalizing:   true,
		Addr:                     addr,
		IsTLS:                    true,
		TLSConfig:                g.cortexTLSConfig,
	}
	return func(c *fiber.Ctx) error {
		req := c.Request()
		resp := c.Response()
		req.Header.Del(fiber.HeaderConnection)
		req.SetRequestURI(utils.UnsafeString(req.RequestURI()))
		if err := hostClient.Do(req, resp); err != nil {
			return err
		}
		resp.Header.Del(fiber.HeaderConnection)
		return nil
	}
}

func (g *Gateway) loadCortexCerts() {
	cortexServerCA := g.config.Spec.Cortex.Certs.ServerCA
	cortexClientCA := g.config.Spec.Cortex.Certs.ClientCA
	cortexClientCert := g.config.Spec.Cortex.Certs.ClientCert
	cortexClientKey := g.config.Spec.Cortex.Certs.ClientKey

	clientCert, err := tls.LoadX509KeyPair(cortexClientCert, cortexClientKey)
	if err != nil {
		log.Fatalf("Failed to load cortex client keypair: %v", err)
	}
	serverCAPool := x509.NewCertPool()
	serverCAData, err := os.ReadFile(cortexServerCA)
	if err != nil {
		log.Fatalf("Failed to read cortex server CA: %v", err)
	}
	if ok := serverCAPool.AppendCertsFromPEM(serverCAData); !ok {
		log.Fatal("Failed to load cortex server CA")
	}
	clientCAPool := x509.NewCertPool()
	clientCAData, err := os.ReadFile(cortexClientCA)
	if err != nil {
		log.Fatalf("Failed to read cortex client CA: %v", err)
	}
	if ok := clientCAPool.AppendCertsFromPEM(clientCAData); !ok {
		log.Fatal("Failed to load cortex client CA")
	}
	g.cortexTLSConfig = &tls.Config{
		ServerName:   "cortex-server",
		Certificates: []tls.Certificate{clientCert},
		ClientCAs:    clientCAPool,
		RootCAs:      serverCAPool,
	}
}

func (g *Gateway) setupCortexRoutes(
	app *fiber.App,
	tenantStore storage.TenantStore,
	rbacStore storage.RBACStore,
) {
	queryFrontend := g.newCortexForwarder(g.config.Spec.Cortex.QueryFrontend.Address)
	alertmanager := g.newCortexForwarder(g.config.Spec.Cortex.Alertmanager.Address)
	ruler := g.newCortexForwarder(g.config.Spec.Cortex.Ruler.Address)
	distributor := g.newCortexForwarder(g.config.Spec.Cortex.Distributor.Address)

	rbacProvider := storage.NewRBACProvider(rbacStore)
	rbacMiddleware := rbac.NewMiddleware(rbacProvider)

	app.Get("/services", queryFrontend)
	app.Get("/ready", queryFrontend)

	// Memberlist
	app.Get("/ring", distributor)
	app.Get("/ruler/ring", ruler)

	// Alertmanager UI
	alertmanagerUi := app.Group("/alertmanager", g.authMiddleware.Handle, rbacMiddleware)
	alertmanagerUi.Get("/alertmanager", alertmanager)

	// Prometheus-compatible API
	promv1 := app.Group("/prometheus/api/v1", g.authMiddleware.Handle, rbacMiddleware)

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

	// Remote-write API
	tenantMiddleware := tenantauth.New(tenantStore)
	v1 := app.Group("/api/v1", tenantMiddleware.Handle)
	v1.Post("/push", distributor)
}

// Returns a complete cert chain including the root CA, and a tls serving cert.
func loadCerts(cacertPath, certPath, keyPath string) (*tls.Certificate, error) {
	data, err := os.ReadFile(cacertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load CA cert: %w", err)
	}
	root, err := util.ParsePEMEncodedCertChain(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA cert: %w", err)
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
