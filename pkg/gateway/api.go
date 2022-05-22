package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/opni/pkg/bootstrap"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util/fwd"
	"github.com/rancher/opni/pkg/util/waitctx"
	"go.uber.org/zap"
)

var (
	httpRequestsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "opni",
		Subsystem: "gateway",
		Name:      "http_requests_total",
		Help:      "Total number of HTTP requests handled by the gateway API",
	})
	apiCollectors = []prometheus.Collector{
		httpRequestsTotal,
	}
)

type GatewayAPIServer struct {
	APIServerOptions
	app            *fiber.App
	conf           *v1beta1.GatewayConfigSpec
	logger         *zap.SugaredLogger
	tlsConfig      *tls.Config
	wait           chan struct{}
	metricsHandler *MetricsEndpointHandler

	reservedPrefixRoutes []string
}

type APIServerOptions struct {
	fiberMiddlewares []FiberMiddleware
	apiExtensions    []APIExtensionPlugin
	metricsPlugins   []MetricsPlugin
}

type APIServerOption func(*APIServerOptions)

func (o *APIServerOptions) Apply(opts ...APIServerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithFiberMiddleware(middlewares ...FiberMiddleware) APIServerOption {
	return func(o *APIServerOptions) {
		o.fiberMiddlewares = append(o.fiberMiddlewares, middlewares...)
	}
}

func WithAPIExtensions(plugins []APIExtensionPlugin) APIServerOption {
	return func(o *APIServerOptions) {
		o.apiExtensions = plugins
	}
}

func WithMetricsPlugins(plugins []MetricsPlugin) APIServerOption {
	return func(o *APIServerOptions) {
		o.metricsPlugins = plugins
	}
}

func NewAPIServer(
	ctx context.Context,
	cfg *v1beta1.GatewayConfigSpec,
	lg *zap.SugaredLogger,
	opts ...APIServerOption,
) *GatewayAPIServer {
	lg = lg.Named("api")

	options := APIServerOptions{}
	options.Apply(opts...)

	app := fiber.New(fiber.Config{
		StrictRouting:           false,
		AppName:                 "Opni Gateway",
		ReduceMemoryUsage:       false,
		Network:                 "tcp4",
		EnableTrustedProxyCheck: len(cfg.TrustedProxies) > 0,
		TrustedProxies:          cfg.TrustedProxies,
		DisableStartupMessage:   true,
	})

	logger.ConfigureAppLogger(app, "gateway")

	tlsConfig, err := loadTLSConfig(cfg)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to load serving cert bundle")
	}
	srv := &GatewayAPIServer{
		APIServerOptions: options,
		app:              app,
		conf:             cfg,
		logger:           lg,
		tlsConfig:        tlsConfig,
		wait:             make(chan struct{}),
		metricsHandler:   NewMetricsEndpointHandler(),
		reservedPrefixRoutes: []string{
			"/monitor",
			"/healthz",
			"/bootstrap",
			"/metrics",
		},
	}

	for _, middleware := range options.fiberMiddlewares {
		app.Use(middleware)
	}

	sampledLog := logger.New(
		logger.WithSampling(&zap.SamplingConfig{
			Initial:    1,
			Thereafter: 0,
		}),
	).Named("api")
	app.Use(func(c *fiber.Ctx) error {
		sampledLog.Debugf("%s %s", c.Method(), c.Request().URI().FullURI())
		httpRequestsTotal.Inc()
		return c.Next()
	})

	if cfg.EnableMonitor {
		app.Get("/monitor", monitor.New())
	}

	app.All("/healthz", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	srv.metricsHandler.MustRegister(apiCollectors...)
	for _, plugin := range options.metricsPlugins {
		srv.metricsHandler.MustRegister(plugin.Typed)
	}

	var lc net.ListenConfig
	if listener, err := lc.Listen(ctx, "tcp4", fmt.Sprintf(":%d", cfg.MetricsPort)); err != nil {
		lg.With(
			zap.Error(err),
		).Error("failed to start metrics listener")
	} else {
		waitctx.Go(ctx, func() {
			if err := srv.metricsHandler.ListenAndServe(listener); err != nil {
				lg.With(
					zap.Error(err),
				).Error("metrics handler stopped")
			}
		})
	}

	go func() {
		for _, plugin := range options.apiExtensions {
			ctx, ca := context.WithTimeout(ctx, 5*time.Second)
			defer ca()
			cfg, err := plugin.Typed.Configure(ctx, apiextensions.NewCertConfig(cfg.Certs))
			if err != nil {
				lg.With(
					zap.String("plugin", plugin.Metadata.Module),
					zap.Error(err),
				).Fatal("failed to configure routes")
			}
			srv.setupPluginRoutes(cfg, plugin.Metadata)
		}
		close(srv.wait)
	}()
	return srv
}

func (s *GatewayAPIServer) Serve(listener net.Listener) error {
	select {
	case <-s.wait:
	case <-time.After(10 * time.Second):
		s.logger.Fatal("failed to start api server: timed out waiting for route setup")
	}
	s.app.Use(default404Handler)

	info, _ := debug.ReadBuildInfo()
	s.logger.With(
		"address", listener.Addr().String(),
		"go-version", info.GoVersion,
		"version", info.Main.Version,
		"sum", info.Main.Sum,
	).Info("gateway server starting")
	return s.app.Listener(listener)
}

func (s *GatewayAPIServer) Shutdown() error {
	return s.app.Shutdown()
}

func (s *GatewayAPIServer) setupPluginRoutes(
	cfg *apiextensions.GatewayAPIExtensionConfig,
	pluginMeta meta.PluginMeta,
) {
	tlsConfig := s.tlsConfig.Clone()
	tlsConfig.InsecureSkipVerify = true
	sampledLogger := logger.New(
		logger.WithSampling(&zap.SamplingConfig{
			Initial:    1,
			Thereafter: 0,
		}),
	).Named("api")
	forwarder := fwd.To(cfg.HttpAddr, fwd.WithTLS(tlsConfig), fwd.WithLogger(sampledLogger))
PREFIXES:
	for _, prefix := range cfg.PathPrefixes {
		// check if the prefix would conflict with any reserved routes
		for _, reserved := range s.reservedPrefixRoutes {
			if strings.HasPrefix(prefix, reserved) {
				s.logger.With(
					"prefix", prefix,
					"existing", prefix,
					"plugin", pluginMeta.Module,
				).Error("requested prefix conflicts with existing route")
				continue PREFIXES
			}
		}
		s.reservedPrefixRoutes = append(s.reservedPrefixRoutes, prefix)
		s.app.Use(prefix, forwarder)
		s.logger.With(
			"route", prefix,
			"plugin", pluginMeta.Module,
		).Debug("configured prefix route for plugin")
	}
}

func (s *GatewayAPIServer) ConfigureBootstrapRoutes(
	storageBackend storage.Backend,
	installer capabilities.Installer,
) {
	limiterCfg := limiter.ConfigDefault
	limiterCfg.Max = 60 // 60 requests per minute
	s.app.Post("/bootstrap/*", limiter.New(limiterCfg), bootstrap.ServerConfig{
		Certificate:         &s.tlsConfig.Certificates[0],
		TokenStore:          storageBackend,
		ClusterStore:        storageBackend,
		KeyringStoreBroker:  storageBackend,
		CapabilityInstaller: installer,
	}.Handle)
}

func default404Handler(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNotFound)
}
