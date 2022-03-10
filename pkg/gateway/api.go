package gateway

import (
	"context"
	"crypto/tls"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/rancher/opni-monitoring/pkg/auth"
	"github.com/rancher/opni-monitoring/pkg/bootstrap"
	"github.com/rancher/opni-monitoring/pkg/capabilities"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/util"
	"github.com/rancher/opni-monitoring/pkg/util/fwd"
	"go.uber.org/zap"
)

type GatewayAPIServer struct {
	APIServerOptions
	app       *fiber.App
	conf      *v1beta1.GatewayConfigSpec
	logger    *zap.SugaredLogger
	tlsConfig *tls.Config
	wait      chan struct{}
}

type APIServerOptions struct {
	fiberMiddlewares []FiberMiddleware
	authMiddleware   auth.NamedMiddleware
	apiExtensions    []APIExtensionPlugin
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

func WithAuthMiddleware(name string) APIServerOption {
	return func(o *APIServerOptions) {
		var err error
		o.authMiddleware, err = auth.GetMiddleware(name)
		if err != nil {
			panic(err)
		}
	}
}

func WithAPIExtensions(plugins []APIExtensionPlugin) APIServerOption {
	return func(o *APIServerOptions) {
		o.apiExtensions = plugins
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

	if options.authMiddleware == nil {
		lg.Fatal("auth middleware is required")
	}

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
	}

	for _, middleware := range options.fiberMiddlewares {
		app.Use(middleware)
	}

	if cfg.EnableMonitor {
		app.Get("/monitor", monitor.New())
	}

	app.All("/healthz", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

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
			srv.setupPluginRoutes(cfg)
		}
		close(srv.wait)
	}()

	// app.Use(default404Handler)
	return srv
}

func (s *GatewayAPIServer) ListenAndServe() error {
	select {
	case <-s.wait:
	case <-time.After(10 * time.Second):
		s.logger.Fatal("failed to start api server: timed out waiting for route setup")
	}
	listener, err := tls.Listen("tcp4",
		s.conf.ListenAddress, s.tlsConfig)
	if err != nil {
		return err
	}
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

func (s *GatewayAPIServer) setupPluginRoutes(cfg *apiextensions.GatewayAPIExtensionConfig) {
	tlsConfig := s.tlsConfig.Clone()
	tlsConfig.InsecureSkipVerify = true
	forwarder := fwd.To(cfg.HttpAddr, fwd.WithTLS(tlsConfig), fwd.WithLogger(s.logger))
	for _, route := range cfg.Routes {
		s.app.Add(route.Method, route.Path, forwarder)
		s.logger.With(
			zap.String("method", route.Method),
			zap.String("path", route.Path),
		).Debug("added route from plugin")
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

func loadTLSConfig(cfg *v1beta1.GatewayConfigSpec) (*tls.Config, error) {
	servingCertBundle, caPool, err := util.LoadServingCertBundle(cfg.Certs)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      caPool,
		Certificates: []tls.Certificate{*servingCertBundle},
	}, nil
}

func default404Handler(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNotFound)
}
