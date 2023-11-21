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
	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/opni/pkg/config/reactive"
	configv1 "github.com/rancher/opni/pkg/config/v1"
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
	certs             reactive.Value
	mgr               *configv1.GatewayConfigManager
	logger            *slog.Logger
	metricsRouter     *gin.Engine
	metricsRegisterer prometheus.Registerer

	routesMu             sync.Mutex
	reservedPrefixRoutes []string
}

func NewHTTPServer(
	ctx context.Context,
	mgr *configv1.GatewayConfigManager,
	lg *slog.Logger,
	pl plugins.LoaderInterface,
) (*GatewayHTTPServer, error) {
	lg = lg.WithGroup("http")

	router := gin.New()
	trustedProxies := mgr.Reactive(configv1.ProtoPath().Dashboard().TrustedProxies())

	trustedProxies.WatchFunc(ctx, func(v protoreflect.Value) {
		list := v.List()
		var items []string
		for i := 0; i < list.Len(); i++ {
			items = append(items, list.Get(i).String())
		}
		router.SetTrustedProxies(items)
	})

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

	pprof.Register(metricsRouter)

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

	metricsHandler := NewMetricsEndpointHandler()
	metricsRouter.GET("/metrics", gin.WrapH(metricsHandler.Handler()))

	certs := mgr.Reactive(protopath.Path(configv1.ProtoPath().Certs()))
	srv := &GatewayHTTPServer{
		router:            router,
		certs:             certs,
		mgr:               mgr,
		logger:            lg,
		metricsRouter:     metricsRouter,
		metricsRegisterer: metricsHandler.reg,
		reservedPrefixRoutes: []string{
			"/healthz",
			"/debug",
			"/metrics",
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
		certs.WatchFunc(ctx, func(v protoreflect.Value) {
			ctx, ca := context.WithTimeout(ctx, 10*time.Second)
			defer ca()
			certs := v.Message().Interface().(*configv1.CertsSpec)
			cfg, err := p.Configure(ctx, certs)
			if err != nil {
				lg.With(
					"plugin", md.Module,
					logger.Err(err),
				).Error("failed to configure routes")
				return
			}
			srv.setupPluginRoutes(cfg, md, certs)
		})
	}))

	return srv, nil
}

func (s *GatewayHTTPServer) ListenAndServe(ctx context.Context) error {
	lg := s.logger

	ctx, ca := context.WithCancelCause(ctx)

	httpListenAddr := s.mgr.Reactive(configv1.ProtoPath().Server().HttpListenAddress())
	metricsListenAddr := s.mgr.Reactive(configv1.ProtoPath().Health().HttpListenAddress())

	e1 := lo.Async(func() error {
		var cancel context.CancelFunc
		var done chan struct{}
		reactive.Bind(ctx, func(v []protoreflect.Value) {
			if cancel != nil {
				cancel()
				<-done
			}
			addr := v[0].String()
			certs, err := v[1].Message().Interface().(*configv1.CertsSpec).AsTlsConfig(tls.NoClientCert)
			if err != nil {
				lg.With(
					logger.Err(err),
				).Error("failed to configure TLS")
				return
			}
			listener, err := tls.Listen("tcp4", addr, certs)
			if err != nil {
				lg.With(
					"address", addr,
					logger.Err(err),
				).Error("failed to start gateway HTTP server")
				return
			}
			lg.With(
				"address", listener.Addr().String(),
			).Info("gateway HTTP server starting")

			var serveContext context.Context
			serveContext, cancel = context.WithCancel(ctx)
			done = make(chan struct{})
			go func() {
				defer close(done)
				if err := util.ServeHandler(serveContext, s.router.Handler(), listener); err != nil {
					lg.With(logger.Err(err)).Warn("gateway HTTP server exited with error")
				}
			}()
		}, httpListenAddr, s.certs)
		<-ctx.Done()
		cancel()
		<-done
		return ctx.Err()
	})

	e2 := lo.Async(func() error {
		var cancel context.CancelFunc
		var done chan struct{}
		metricsListenAddr.WatchFunc(ctx, func(v protoreflect.Value) {
			if cancel != nil {
				cancel()
				<-done
			}
			addr := v.String()
			listener, err := net.Listen("tcp4", addr)
			if err != nil {
				lg.With(
					"address", addr,
					logger.Err(err),
				).Error("failed to start metrics HTTP server")
				return
			}
			lg.With(
				"address", listener.Addr().String(),
			).Info("metrics HTTP server starting")

			var serveContext context.Context
			serveContext, cancel = context.WithCancel(ctx)
			done = make(chan struct{})
			go func() {
				defer close(done)
				if err := util.ServeHandler(serveContext, s.metricsRouter.Handler(), listener); err != nil {
					lg.With(logger.Err(err)).Warn("metrics HTTP server exited with error")
				}
			}()
		})
		<-ctx.Done()
		cancel()
		<-done
		return ctx.Err()
	})

	util.WaitAll(ctx, ca, e1, e2)
	return context.Cause(ctx)
}

func (s *GatewayHTTPServer) setupPluginRoutes(
	cfg *apiextensions.HTTPAPIExtensionConfig,
	pluginMeta meta.PluginMeta,
	certs *configv1.CertsSpec,
) {
	s.routesMu.Lock()
	defer s.routesMu.Unlock()
	tlsConfig, err := certs.AsTlsConfig(tls.NoClientCert)
	if err != nil {
		s.logger.With(
			logger.Err(err),
		).Error("failed to configure TLS for plugin routes")
		return
	}
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
