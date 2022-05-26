package gateway

import (
	"context"
	"crypto/tls"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/plugins/types"
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

type GatewayHTTPServer struct {
	app            *fiber.App
	conf           *v1beta1.GatewayConfigSpec
	logger         *zap.SugaredLogger
	tlsConfig      *tls.Config
	metricsHandler *MetricsEndpointHandler

	routesMu             sync.Mutex
	reservedPrefixRoutes []string
}

func NewHTTPServer(
	ctx context.Context,
	cfg *v1beta1.GatewayConfigSpec,
	lg *zap.SugaredLogger,
	pl plugins.LoaderInterface,
) *GatewayHTTPServer {
	lg = lg.Named("http")

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

	tlsConfig, _, err := loadTLSConfig(cfg)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to load serving cert bundle")
	}
	srv := &GatewayHTTPServer{
		app:            app,
		conf:           cfg,
		logger:         lg,
		tlsConfig:      tlsConfig,
		metricsHandler: NewMetricsEndpointHandler(cfg.Metrics),
		reservedPrefixRoutes: []string{
			cfg.Metrics.GetPath(),
			"/healthz",
		},
	}

	sampledLog := logger.New(
		logger.WithSampling(&zap.SamplingConfig{
			Initial:    1,
			Thereafter: 0,
		}),
	).Named("http")
	app.Use(func(c *fiber.Ctx) error {
		sampledLog.Debugf("%s %s", c.Method(), c.Request().URI().FullURI())
		httpRequestsTotal.Inc()
		return c.Next()
	})

	app.All("/healthz", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	srv.metricsHandler.MustRegister(apiCollectors...)
	pl.Hook(hooks.OnLoad(func(p types.MetricsPlugin) {
		srv.metricsHandler.MustRegister(p)
	}))

	pl.Hook(hooks.OnLoadM(func(p types.GatewayAPIExtensionPlugin, md meta.PluginMeta) {
		ctx, ca := context.WithTimeout(ctx, 10*time.Second)
		defer ca()
		cfg, err := p.Configure(ctx, apiextensions.NewCertConfig(cfg.Certs))
		if err != nil {
			lg.With(
				zap.String("plugin", md.Module),
				zap.Error(err),
			).Fatal("failed to configure routes")
		}
		srv.setupPluginRoutes(cfg, md)
	}))

	return srv
}

func (s *GatewayHTTPServer) ListenAndServe(ctx waitctx.RestrictiveContext) error {
	lg := s.logger

	waitctx.Go(ctx, func() {
		if err := s.metricsHandler.ListenAndServe(ctx); err != nil {
			lg.With(
				zap.Error(err),
			).Error("metrics handler exited with error")
		}
	})

	listener, err := tls.Listen("tcp4", s.conf.HTTPListenAddress, s.tlsConfig)
	if err != nil {
		return err
	}

	lg.With(
		"address", listener.Addr().String(),
	).Info("gateway HTTP server starting")

	waitctx.Go(ctx, func() {
		<-ctx.Done()
		s.app.Shutdown()
	})
	return s.app.Listener(listener)
}

func (s *GatewayHTTPServer) setupPluginRoutes(
	cfg *apiextensions.GatewayAPIExtensionConfig,
	pluginMeta meta.PluginMeta,
) {
	s.routesMu.Lock()
	defer s.routesMu.Unlock()
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
					"existing", reserved,
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
