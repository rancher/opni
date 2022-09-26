package gateway

import (
	"context"
	"crypto/tls"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

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
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/trace"
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
	router         *gin.Engine
	conf           *v1beta1.GatewayConfigSpec
	logger         *zap.SugaredLogger
	tlsConfig      *tls.Config
	metricsHandler *MetricsEndpointHandler
	pprofHandler   *PprofEndpointHandler
	tracer         trace.Tracer

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

	router.GET("/healthz", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	tlsConfig, _, err := loadTLSConfig(cfg)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to load serving cert bundle")
	}
	srv := &GatewayHTTPServer{
		router:         router,
		conf:           cfg,
		logger:         lg,
		tlsConfig:      tlsConfig,
		metricsHandler: NewMetricsEndpointHandler(cfg.Metrics),
		pprofHandler:   NewPprofEndpointHandler(cfg.Profiling),
		reservedPrefixRoutes: []string{
			cfg.Metrics.GetPath(),
			"/healthz",
		},
	}

	srv.metricsHandler.MustRegister(apiCollectors...)
	pl.Hook(hooks.OnLoad(func(p types.MetricsPlugin) {
		srv.metricsHandler.MustRegister(p)
	}))

	pl.Hook(hooks.OnLoadM(func(p types.HTTPAPIExtensionPlugin, md meta.PluginMeta) {
		ctx, ca := context.WithTimeout(ctx, 10*time.Second)
		defer ca()
		cfg, err := p.Configure(ctx, apiextensions.NewCertConfig(cfg.Certs))
		if err != nil {
			lg.With(
				zap.String("plugin", md.Module),
				zap.Error(err),
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

	lg.With(
		"address", listener.Addr().String(),
	).Info("gateway HTTP server starting")

	ctx, ca := context.WithCancel(ctx)

	e1 := lo.Async(func() error {
		return util.ServeHandler(ctx, s.router.Handler(), listener)
	})

	e2 := lo.Async(func() error {
		return s.metricsHandler.ListenAndServe(ctx)
	})

	e3 := lo.Async(func() error {
		if s.conf.Profiling.Enabled {
			return s.pprofHandler.ListenAndServe(ctx)
		}
		return nil
	})

	return util.WaitAll(ctx, ca, e1, e2, e3)
}

func (s *GatewayHTTPServer) setupPluginRoutes(
	cfg *apiextensions.HTTPAPIExtensionConfig,
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
	forwarder := fwd.To(cfg.HttpAddr,
		fwd.WithTLS(tlsConfig),
		fwd.WithLogger(sampledLogger),
		fwd.WithDestHint(path.Base(pluginMeta.BinaryPath)),
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
