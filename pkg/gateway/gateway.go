package gateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/utils"
	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni-monitoring/pkg/auth/cluster"
	"github.com/rancher/opni-monitoring/pkg/bootstrap"
	"github.com/rancher/opni-monitoring/pkg/config"
	"github.com/rancher/opni-monitoring/pkg/config/meta"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/system"
	pluginmeta "github.com/rancher/opni-monitoring/pkg/plugins/meta"
	"github.com/rancher/opni-monitoring/pkg/rbac"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/storage/etcd"
	"github.com/rancher/opni-monitoring/pkg/util"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Gateway struct {
	GatewayOptions
	config              *config.GatewayConfig
	app                 *fiber.App
	logger              *zap.SugaredLogger
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

// If no lifecycler is provided, fall back to one with limited functionality
type fallbackLifecycler struct {
	objects meta.ObjectList
}

func (l *fallbackLifecycler) ReloadC() (chan struct{}, error) {
	return nil, status.Error(codes.Unavailable, "lifecycler not available")
}

func (l *fallbackLifecycler) GetObjectList() (meta.ObjectList, error) {
	return l.objects, nil
}
func (l *fallbackLifecycler) UpdateObjectList(objects meta.ObjectList) error {
	return status.Error(codes.Unavailable, "lifecycler not available")
}

func NewGateway(conf *config.GatewayConfig, opts ...GatewayOption) *Gateway {
	options := GatewayOptions{
		fiberMiddlewares: []FiberMiddleware{},
		lifecycler: &fallbackLifecycler{
			objects: meta.ObjectList{conf},
		},
	}
	options.Apply(opts...)

	lg := logger.New().Named("gateway")

	loadPlugins(conf.Spec.Plugins)

	var tokenStore storage.TokenStore
	var clusterStore storage.ClusterStore
	var rbacStore storage.RBACStore
	switch conf.Spec.Storage.Type {
	case v1beta1.StorageTypeEtcd:
		options := conf.Spec.Storage.Etcd
		if options == nil {
			lg.Fatal("etcd storage options are not set")
		} else {
			store := etcd.NewEtcdStore(conf.Spec.Storage.Etcd,
				etcd.WithNamespace("gateway"),
			)
			tokenStore = store
			clusterStore = store
			rbacStore = store
		}
	default:
		lg.With(
			"type", conf.Spec.Storage.Type,
		).Fatal("unknown storage type")
	}

	conf.Spec.SetDefaults()

	if options.authMiddleware == nil {
		lg.Fatal("auth middleware is required")
	}

	app := fiber.New(fiber.Config{
		Prefork:                 options.prefork,
		StrictRouting:           false,
		AppName:                 "Opni Gateway",
		ReduceMemoryUsage:       false,
		Network:                 "tcp4",
		EnableTrustedProxyCheck: len(conf.Spec.TrustedProxies) > 0,
		TrustedProxies:          conf.Spec.TrustedProxies,
		DisableStartupMessage:   true,
	})
	logger.ConfigureApp(app, lg)

	for _, middleware := range options.fiberMiddlewares {
		app.Use(middleware)
	}

	if conf.Spec.EnableMonitor {
		app.Get("/monitor", monitor.New())
	}

	app.All("/healthz", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	servingCertBundle, err := loadServingCertBundle(conf.Spec.Certs)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to load serving cert bundle")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*servingCertBundle},
	}

	apiExtensionPlugins := plugins.DispenseAll(apiextensions.ManagementAPIExtensionPluginID)

	mgmtSrv := management.NewServer(&conf.Spec.Management,
		management.TokenStore(tokenStore),
		management.ClusterStore(clusterStore),
		management.RBACStore(rbacStore),
		management.TLSConfig(tlsConfig),
		management.APIExtensions(apiExtensionPlugins),
		management.Lifecycler(options.lifecycler),
	)

	g := &Gateway{
		GatewayOptions:    options,
		config:            conf,
		app:               app,
		logger:            lg,
		managementServer:  mgmtSrv,
		tlsConfig:         tlsConfig,
		servingCertBundle: servingCertBundle,
	}
	g.loadCortexCerts()
	g.setupCortexRoutes(app, rbacStore, clusterStore)

	app.Post("/bootstrap/*", bootstrap.ServerConfig{
		Certificate:  servingCertBundle,
		TokenStore:   tokenStore,
		ClusterStore: clusterStore,
	}.Handle).Use(limiter.New()) // Limit requests to 5 per minute

	app.Use(default404Handler)

	return g
}

func (g *Gateway) Listen() error {
	g.managementCtx, g.managementCtxCancel =
		context.WithCancel(context.Background())
	go func() {
		if err := g.managementServer.ListenAndServe(g.managementCtx); err != nil {
			g.logger.Error(err)
		}
	}()

	systemPlugins := plugins.DispenseAll(system.SystemPluginID)
	g.logger.Infof("serving management api for %d system plugins", len(systemPlugins))
	for _, systemPlugin := range systemPlugins {
		srv := systemPlugin.Raw.(system.SystemPluginServer)
		go srv.ServeManagementAPI(g.managementServer)
	}

	if g.servingCertBundle == nil {
		return g.app.Listen(g.config.Spec.ListenAddress)
	}

	listener, err := tls.Listen(g.app.Config().Network,
		g.config.Spec.ListenAddress, g.tlsConfig)
	if err != nil {
		return err
	}
	g.logger.With(
		"address", listener.Addr().String(),
	).Info("gateway server starting")
	return g.app.Listener(listener)
}

func (g *Gateway) Shutdown() error {
	g.managementCtxCancel()
	plugin.CleanupClients()
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
			return fmt.Errorf("cortex error: %w", err)
		}
		resp.Header.Del(fiber.HeaderConnection)
		if resp.StatusCode() != http.StatusOK {
			resp.Header.SetStatusMessage([]byte(fmt.Sprintf("cortex: %s", resp.Header.StatusMessage())))
		}
		return nil
	}
}

func (g *Gateway) loadCortexCerts() {
	lg := g.logger
	cortexServerCA := g.config.Spec.Cortex.Certs.ServerCA
	cortexClientCA := g.config.Spec.Cortex.Certs.ClientCA
	cortexClientCert := g.config.Spec.Cortex.Certs.ClientCert
	cortexClientKey := g.config.Spec.Cortex.Certs.ClientKey

	clientCert, err := tls.LoadX509KeyPair(cortexClientCert, cortexClientKey)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to load cortex client keypair")
	}
	serverCAPool := x509.NewCertPool()
	serverCAData, err := os.ReadFile(cortexServerCA)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatalf("failed to read cortex server CA")
	}
	if ok := serverCAPool.AppendCertsFromPEM(serverCAData); !ok {
		lg.Fatal("failed to load cortex server CA")
	}
	clientCAPool := x509.NewCertPool()
	clientCAData, err := os.ReadFile(cortexClientCA)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatalf("failed to read cortex client CA")
	}
	if ok := clientCAPool.AppendCertsFromPEM(clientCAData); !ok {
		lg.Fatal("failed to load cortex client CA")
	}
	g.cortexTLSConfig = &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		ClientCAs:    clientCAPool,
		RootCAs:      serverCAPool,
	}
}

func (g *Gateway) setupCortexRoutes(
	app *fiber.App,
	rbacStore storage.RBACStore,
	clusterStore storage.ClusterStore,
) {
	queryFrontend := g.newCortexForwarder(g.config.Spec.Cortex.QueryFrontend.Address)
	alertmanager := g.newCortexForwarder(g.config.Spec.Cortex.Alertmanager.Address)
	ruler := g.newCortexForwarder(g.config.Spec.Cortex.Ruler.Address)
	distributor := g.newCortexForwarder(g.config.Spec.Cortex.Distributor.Address)

	rbacProvider := storage.NewRBACProvider(rbacStore, clusterStore)
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
	clusterMiddleware := cluster.New(clusterStore)
	v1 := app.Group("/api/v1", clusterMiddleware.Handle)
	v1.Post("/push", distributor)
}

// Returns a complete cert chain including the root CA, and a tls serving cert.
func loadServingCertBundle(certsSpec v1beta1.CertsSpec) (*tls.Certificate, error) {
	var caCertData, servingCertData, servingKeyData []byte
	switch {
	case certsSpec.CACert != nil:
		data, err := os.ReadFile(*certsSpec.CACert)
		if err != nil {
			return nil, fmt.Errorf("failed to load CA cert: %w", err)
		}
		caCertData = data
	case certsSpec.CACertData != nil:
		caCertData = []byte(*certsSpec.CACertData)
	default:
		return nil, errors.New("no CA cert configured")
	}
	switch {
	case certsSpec.ServingCert != nil:
		data, err := os.ReadFile(*certsSpec.ServingCert)
		if err != nil {
			return nil, fmt.Errorf("failed to load serving cert: %w", err)
		}
		servingCertData = data
	case certsSpec.ServingCertData != nil:
		servingCertData = []byte(*certsSpec.ServingCertData)
	default:
		return nil, errors.New("no serving cert configured")
	}
	switch {
	case certsSpec.ServingKey != nil:
		data, err := os.ReadFile(*certsSpec.ServingKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load serving key: %w", err)
		}
		servingKeyData = data
	case certsSpec.ServingKeyData != nil:
		servingKeyData = []byte(*certsSpec.ServingKeyData)
	default:
		return nil, errors.New("no serving key configured")
	}

	rootCA, err := util.ParsePEMEncodedCert(caCertData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CA cert: %w", err)
	}
	servingCert, err := tls.X509KeyPair(servingCertData, servingKeyData)
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

func loadPlugins(conf v1beta1.PluginsSpec) {
	for _, dir := range conf.Dirs {
		pluginPaths, err := plugin.Discover("plugin_*", dir)
		if err != nil {
			continue
		}
		for _, p := range pluginPaths {
			cc := plugins.ClientConfig(pluginmeta.PluginMeta{
				Path: p,
			}, plugins.Scheme)
			plugins.Load(cc)
		}
	}
}
