package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"log/slog"

	"github.com/gin-contrib/pprof"
	"github.com/ttacon/chalk"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/plugins/types"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/fwd"
	"github.com/samber/lo"
	slogsampling "github.com/samber/slog-sampling"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
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
	router            *gin.Engine
	conf              *v1beta1.GatewayConfigSpec
	logger            *slog.Logger
	tlsConfig         *tls.Config
	metricsRouter     *gin.Engine
	metricsRegisterer prometheus.Registerer

	routesMu             sync.Mutex
	reservedPrefixRoutes []string
}

func NewHTTPServer(
	ctx context.Context,
	cfg *v1beta1.GatewayConfigSpec,
	lg *slog.Logger,
	pl plugins.LoaderInterface,
) *GatewayHTTPServer {
	lg = lg.WithGroup("http")

	router := gin.New()
	router.SetTrustedProxies(cfg.TrustedProxies)

	router.Use(
		logger.GinLogger(lg),
		gin.Recovery(),
		otelgin.Middleware("gateway"),
		func(c *gin.Context) {
			httpRequestsTotal.Inc()
		},
	)

	var healthz atomic.Int32
	healthz.Store(http.StatusServiceUnavailable)

	metricsRouter := gin.New()
	metricsRouter.GET("/healthz", func(c *gin.Context) {
		c.Status(int(healthz.Load()))
	})

	pl.Hook(hooks.OnLoadingCompleted(func(i int) {
		healthz.Store(http.StatusOK)
	}))

	if cfg.Profiling.Path != "" {
		pprof.Register(metricsRouter, cfg.Profiling.Path)
	} else {
		pprof.Register(metricsRouter)
	}

	metricsRouter.POST("/debug/profiles/:name/enable", func(c *gin.Context) {
		name := c.Param("name")
		switch name {
		case "block":
			runtime.SetBlockProfileRate(1)
		case "mutex":
			runtime.SetMutexProfileFraction(1)
		default:
			c.Status(http.StatusBadRequest)
			return
		}
		fmt.Printf(chalk.Red.Color(`
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
@ %s PROFILING STARTED - DO NOT LEAVE THIS ENABLED! @
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
`[1:]), strings.ToUpper(name))
	})

	metricsRouter.POST("/debug/profiles/:name/disable", func(c *gin.Context) {
		name := c.Param("name")
		switch name {
		case "block":
			runtime.SetBlockProfileRate(0)
		case "mutex":
			runtime.SetMutexProfileFraction(0)
		default:
			c.Status(http.StatusBadRequest)
			return
		}
		fmt.Printf(chalk.Green.Color("%s profiling stopped\n"), name)
	})

	metricsHandler := NewMetricsEndpointHandler(cfg.Metrics)
	metricsRouter.GET(cfg.Metrics.GetPath(), gin.WrapH(metricsHandler.Handler()))

	tlsConfig, _, err := httpTLSConfig(cfg)
	if err != nil {
		lg.With(
			logger.Err(err),
		).Error("failed to load serving cert bundle")
		panic("failed to load serving cert bundle")
	}
	srv := &GatewayHTTPServer{
		router:            router,
		conf:              cfg,
		logger:            lg,
		tlsConfig:         tlsConfig,
		metricsRouter:     metricsRouter,
		metricsRegisterer: metricsHandler.reg,
		reservedPrefixRoutes: []string{
			cfg.Metrics.GetPath(),
			"/healthz",
		},
	}

	srv.metricsRegisterer.MustRegister(apiCollectors...)

	exporter, err := otelprom.New(
		otelprom.WithRegisterer(prometheus.WrapRegistererWithPrefix("opni_gateway_", srv.metricsRegisterer)),
		otelprom.WithoutScopeInfo(),
		otelprom.WithoutTargetInfo(),
	)
	if err != nil {
		lg.With(
			logger.Err(err),
		).Error("failed to create prometheus exporter")
		panic("failed to create prometheus exporter")
	}

	// We are using remote producers, but we need to register the exporter locally
	// to prevent errors
	metric.NewMeterProvider(
		metric.WithReader(exporter),
	)

	pl.Hook(hooks.OnLoad(func(p types.MetricsPlugin) {
		exporter.RegisterProducer(p)
	}))

	pl.Hook(hooks.OnLoadM(func(p types.HTTPAPIExtensionPlugin, md meta.PluginMeta) {
		ctx, ca := context.WithTimeout(ctx, 10*time.Second)
		defer ca()
		cfg, err := p.Configure(ctx, apiextensions.NewCertConfig(cfg.Certs))
		if err != nil {
			lg.With(
				"plugin", md.Module,
				logger.Err(err),
			).Error("failed to configure routes")
			return
		}
		srv.setupPluginRoutes(cfg, md)
	}))

	return srv
}

func (s *GatewayHTTPServer) ListenAndServe(ctx context.Context) error {
	lg := s.logger

	listener, err := tls.Listen("tcp4", s.conf.HTTPListenAddress, s.tlsConfig)
	if err != nil {
		return err
	}

	metricsListener, err := net.Listen("tcp4", s.conf.MetricsListenAddress)
	if err != nil {
		return err
	}

	lg.With(
		"api", listener.Addr().String(),
		"metrics", metricsListener.Addr().String(),
	).Info("gateway HTTP server starting")

	ctx, ca := context.WithCancelCause(ctx)

	e1 := lo.Async(func() error {
		return util.ServeHandler(ctx, s.router.Handler(), listener)
	})

	e2 := lo.Async(func() error {
		return util.ServeHandler(ctx, s.metricsRouter.Handler(), metricsListener)
	})

	return util.WaitAll(ctx, ca, e1, e2)
}

func (s *GatewayHTTPServer) setupPluginRoutes(
	cfg *apiextensions.HTTPAPIExtensionConfig,
	pluginMeta meta.PluginMeta,
) {
	s.routesMu.Lock()
	defer s.routesMu.Unlock()
	tlsConfig := s.tlsConfig.Clone()
	sampledLogger := logger.New(
		logger.WithSampling(&slogsampling.ThresholdSamplingOption{Threshold: 1, Tick: logger.NoRepeatInterval, Rate: 0})).WithGroup("api")

	forwarder := fwd.To(cfg.HttpAddr,
		fwd.WithTLS(tlsConfig),
		fwd.WithLogger(sampledLogger),
		fwd.WithDestHint(pluginMeta.Filename()),
	)
ROUTES:
	for _, route := range cfg.Routes {
		for _, reservedPrefix := range s.reservedPrefixRoutes {
			if strings.HasPrefix(route.Path, reservedPrefix) {
				s.logger.With(
					"route", route.Method+" "+route.Path,
					"plugin", pluginMeta.Module,
				).Warn("skipping route for plugin as it conflicts with a reserved prefix")
				continue ROUTES
			}
		}
		s.logger.With(
			"route", route.Method+" "+route.Path,
			"plugin", pluginMeta.Module,
		).Debug("configured route for plugin")
		s.router.Handle(route.Method, route.Path, forwarder)
	}
}
