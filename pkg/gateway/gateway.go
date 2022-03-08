package gateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"runtime/debug"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/monitor"
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
	gatewayext "github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions/gateway"
	managementext "github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/system"
	pluginmeta "github.com/rancher/opni-monitoring/pkg/plugins/meta"
	"github.com/rancher/opni-monitoring/pkg/rbac"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/storage/crds"
	"github.com/rancher/opni-monitoring/pkg/storage/etcd"
	"github.com/rancher/opni-monitoring/pkg/storage/secrets"
	"github.com/rancher/opni-monitoring/pkg/util"
	"github.com/rancher/opni-monitoring/pkg/util/fwd"
	"github.com/rancher/opni-monitoring/pkg/waitctx"
	"github.com/rancher/opni-monitoring/pkg/webui"
	"go.uber.org/zap"
	"golang.org/x/mod/module"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Gateway struct {
	GatewayOptions
	config            *config.GatewayConfig
	ctx               context.Context
	app               *fiber.App
	logger            *zap.SugaredLogger
	managementServer  *management.Server
	tlsConfig         *tls.Config
	cortexTLSConfig   *tls.Config
	servingCertBundle *tls.Certificate
	pluginLoader      *plugins.PluginLoader
	kvBroker          storage.KeyValueStoreBroker
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

func NewGateway(ctx context.Context, conf *config.GatewayConfig, opts ...GatewayOption) *Gateway {
	options := GatewayOptions{
		fiberMiddlewares: []FiberMiddleware{},
		lifecycler: &fallbackLifecycler{
			objects: meta.ObjectList{conf},
		},
	}
	options.Apply(opts...)

	lg := logger.New().Named("gateway")
	pluginLoader := plugins.NewPluginLoader()

	loadPlugins(pluginLoader, conf.Spec.Plugins)

	storageBackend := storage.CompositeBackend{}

	switch conf.Spec.Storage.Type {
	case v1beta1.StorageTypeEtcd:
		options := conf.Spec.Storage.Etcd
		if options == nil {
			lg.Fatal("etcd storage options are not set")
		} else {
			store := etcd.NewEtcdStore(ctx, conf.Spec.Storage.Etcd,
				etcd.WithPrefix("gateway"),
			)
			storageBackend.Use(store)
		}
	case v1beta1.StorageTypeCRDs:
		options := conf.Spec.Storage.CustomResources
		crdOpts := []crds.CRDStoreOption{}
		secOpts := []secrets.SecretsStoreOption{}
		if options != nil {
			crdOpts = append(crdOpts, crds.WithNamespace(options.Namespace))
			secOpts = append(secOpts, secrets.WithNamespace(options.Namespace))
		}
		crdStore := crds.NewCRDStore(crdOpts...)
		secretStore := secrets.NewSecretsStore(secOpts...)
		storageBackend.Use(crdStore)
		storageBackend.Use(secretStore)
	case v1beta1.StorageTypeSecret:
		lg.With(
			"type", conf.Spec.Storage.Type,
		).Fatal("secret storage is only supported for agent keyrings")
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
	logger.ConfigureAppLogger(app, "gateway")

	for _, middleware := range options.fiberMiddlewares {
		app.Use(middleware)
	}

	if conf.Spec.EnableMonitor {
		app.Get("/monitor", monitor.New())
	}

	app.All("/healthz", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	servingCertBundle, caPool, err := util.LoadServingCertBundle(conf.Spec.Certs)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to load serving cert bundle")
	}

	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      caPool,
		Certificates: []tls.Certificate{*servingCertBundle},
	}

	apiExtensionPlugins := pluginLoader.DispenseAll(managementext.ManagementAPIExtensionPluginID)

	mgmtSrv := management.NewServer(ctx, &conf.Spec.Management,
		management.StorageBackend(storageBackend),
		management.TLSConfig(tlsConfig),
		management.APIExtensions(apiExtensionPlugins),
		management.Lifecycler(options.lifecycler),
	)

	g := &Gateway{
		GatewayOptions:    options,
		ctx:               ctx,
		config:            conf,
		app:               app,
		logger:            lg,
		managementServer:  mgmtSrv,
		tlsConfig:         tlsConfig,
		servingCertBundle: servingCertBundle,
		pluginLoader:      pluginLoader,
		kvBroker:          storageBackend,
	}
	g.loadCortexCerts()
	g.setupCortexRoutes(storageBackend, storageBackend)

	gatewayExtensionPlugins := plugins.DispenseAllAs[apiextensions.GatewayAPIExtensionClient](
		pluginLoader, gatewayext.GatewayAPIExtensionPluginID)

	for _, plugin := range gatewayExtensionPlugins {
		cfg, err := plugin.Typed.Configure(ctx, apiextensions.NewCertConfig(g.config.Spec.Certs))
		if err != nil {
			lg.With(
				zap.String("plugin", plugin.Metadata.Module),
				zap.Error(err),
			).Fatal("failed to configure routes")
		}
		g.setupPluginRoutes(cfg)
	}

	app.Post("/bootstrap/*", bootstrap.ServerConfig{
		Certificate:        servingCertBundle,
		TokenStore:         storageBackend,
		ClusterStore:       storageBackend,
		KeyringStoreBroker: storageBackend,
	}.Handle).Use(limiter.New()) // Limit requests to 5 per minute

	app.Use(default404Handler)

	waitctx.Go(ctx, func() {
		<-ctx.Done()
		lg.Info("shutting down plugins")
		plugin.CleanupClients()
	})

	return g
}

func (g *Gateway) setupPluginRoutes(cfg *apiextensions.GatewayAPIExtensionConfig) {
	lg := g.logger
	forwarder := fwd.To(cfg.HttpAddr, fwd.WithTLS(g.tlsConfig))
	for _, route := range cfg.Routes {
		g.app.Add(route.Method, route.Path, forwarder)
		lg.With(
			zap.String("method", route.Method),
			zap.String("path", route.Path),
		).Info("added route from plugin")
	}
}

func (g *Gateway) Listen() error {
	lg := g.logger
	go func() {
		if err := g.managementServer.ListenAndServe(); err != nil {
			g.logger.With(
				zap.Error(err),
			).Warn("management server stopped")
		}
	}()
	waitctx.Go(g.ctx, func() {
		<-g.ctx.Done()
		lg.Info("shutting down gateway api")
		if err := g.app.Shutdown(); err != nil {
			lg.With(
				zap.Error(err),
			).Error("error shutting down gateway api")
		}
	})

	webuiSrv, err := webui.NewWebUIServer(g.config)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Warn("not starting web ui server")
	} else {
		go func() {
			if err := webuiSrv.ListenAndServe(); err != nil {
				lg.With(
					zap.Error(err),
				).Warn("ui server stopped")
			}
		}()
		waitctx.Go(g.ctx, func() {
			<-g.ctx.Done()
			lg.Info("shutting down ui server")
			if err := webuiSrv.Shutdown(context.Background()); err != nil {
				lg.With(
					zap.Error(err),
				).Error("error shutting down ui server")
			}
		})
	}

	systemPlugins := g.pluginLoader.DispenseAll(system.SystemPluginID)
	g.logger.Infof("serving management api for %d system plugins", len(systemPlugins))
	for _, systemPlugin := range systemPlugins {
		srv := systemPlugin.Raw.(system.SystemPluginServer)
		ns := systemPlugin.Metadata.Module
		if err := module.CheckPath(ns); err != nil {
			g.logger.With(
				zap.String("namespace", ns),
				zap.Error(err),
			).Warn("system plugin module name is invalid")
			continue
		}
		store, err := g.kvBroker.KeyValueStore(ns)
		if err != nil {
			return err
		}
		go srv.ServeManagementAPI(g.managementServer)
		go srv.ServeKeyValueStore(store)
	}

	if g.servingCertBundle == nil {
		return g.app.Listen(g.config.Spec.ListenAddress)
	}

	listener, err := tls.Listen(g.app.Config().Network,
		g.config.Spec.ListenAddress, g.tlsConfig)
	if err != nil {
		return err
	}
	info, _ := debug.ReadBuildInfo()
	g.logger.With(
		"address", listener.Addr().String(),
		"go-version", info.GoVersion,
		"version", info.Main.Version,
		"sum", info.Main.Sum,
	).Info("gateway server starting")
	return g.app.Listener(listener)
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
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{clientCert},
		ClientCAs:    clientCAPool,
		RootCAs:      serverCAPool,
	}
}

func (g *Gateway) setupCortexRoutes(
	rbacStore storage.RBACStore,
	clusterStore storage.ClusterStore,
) {
	queryFrontend := fwd.To(g.config.Spec.Cortex.QueryFrontend.Address, fwd.WithTLS(g.cortexTLSConfig), fwd.WithName("cortex.query-frontend"))
	alertmanager := fwd.To(g.config.Spec.Cortex.Alertmanager.Address, fwd.WithTLS(g.cortexTLSConfig), fwd.WithName("cortex.alertmanager"))
	ruler := fwd.To(g.config.Spec.Cortex.Ruler.Address, fwd.WithTLS(g.cortexTLSConfig), fwd.WithName("cortex.ruler"))
	distributor := fwd.To(g.config.Spec.Cortex.Distributor.Address, fwd.WithTLS(g.cortexTLSConfig), fwd.WithName("cortex.distributor"))

	rbacProvider := storage.NewRBACProvider(rbacStore, clusterStore)
	rbacMiddleware := rbac.NewMiddleware(rbacProvider)

	g.app.Get("/services", queryFrontend)
	g.app.Get("/ready", queryFrontend)

	// Memberlist
	g.app.Get("/ring", distributor)
	g.app.Get("/ruler/ring", ruler)

	// Alertmanager UI
	alertmanagerUi := g.app.Group("/alertmanager", g.authMiddleware.Handle, rbacMiddleware)
	alertmanagerUi.Get("/alertmanager", alertmanager)

	// Prometheus-compatible API
	promv1 := g.app.Group("/prometheus/api/v1", g.authMiddleware.Handle, rbacMiddleware)

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
	v1 := g.app.Group("/api/v1", clusterMiddleware.Handle)
	v1.Post("/push", distributor)
}

func loadPlugins(loader *plugins.PluginLoader, conf v1beta1.PluginsSpec) {
	for _, dir := range conf.Dirs {
		pluginPaths, err := plugin.Discover("plugin_*", dir)
		if err != nil {
			continue
		}
		for _, p := range pluginPaths {
			md, err := pluginmeta.ReadMetadata(p)
			if err != nil {
				loader.Logger.With(
					zap.String("plugin", p),
				).Error("failed to read plugin metadata", zap.Error(err))
				continue
			}
			cc := plugins.ClientConfig(md, plugins.ClientScheme)
			loader.Load(md, cc)
		}
	}
}
