package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/rancher/opni/pkg/alerting/condition"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/opni/pkg/alerting"
	"github.com/rancher/opni/pkg/alerting/shared"
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
	router            *gin.Engine
	conf              *v1beta1.GatewayConfigSpec
	logger            *zap.SugaredLogger
	tlsConfig         *tls.Config
	metricsRouter     *gin.Engine
	metricsRegisterer prometheus.Registerer

	routesMu             sync.Mutex
	reservedPrefixRoutes []string
}

func NewHTTPServer(
	ctx context.Context,
	cfg *v1beta1.GatewayConfigSpec,
	lg *zap.SugaredLogger,
	pl plugins.LoaderInterface,
	alertProvider *alerting.Provider, // need pointer to interface so that it updates to correct impl on change
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

	handlerName := cfg.Alerting.ManagementHookHandler
	// request body will be in the form of AM webhook payload :
	// https://prometheus.io/docs/alerting/latest/configuration/#webhook_config
	//
	// Note :
	//    Webhooks are assumed to respond with 2xx response codes on a successful
	//	  request and 5xx response codes are assumed to be recoverable.
	// therefore, non-recoverable errors should have error codes 3XX and 4XX
	router.POST(handlerName, func(c *gin.Context) {
		if alerting.IsNil(alertProvider) {
			c.Status(http.StatusConflict)
			return
		}
		b, err := io.ReadAll(c.Request.Body)
		if err != nil {
			lg.With("handler", handlerName).Error(
				fmt.Sprintf("failed to read request body %s", err),
			)
			c.Status(http.StatusBadRequest)
			return
		}
		annotations, err := condition.ParseCortexPayloadBytes(b)
		if err != nil {
			lg.With("handler", handlerName).Error(
				fmt.Sprintf("failed to read request body %s", err),
			)
			c.Status(http.StatusBadRequest)
			return
		}
		//
		opniAlertingRequests, errors := condition.ParseAlertManagerWebhookPayload(annotations)
		if len(opniAlertingRequests) != len(errors) {
			// this would be a non-recoverable interval server error since this means the code
			// is written wrong => panic?
			panic(errors)
		}
		var anyErrors []error
		for _, opniAlertingRequest := range opniAlertingRequests {
			resp, err := alerting.DoTrigger(*alertProvider, ctx, opniAlertingRequest)
			if err != nil && err != shared.AlertingErrNotImplementedNOOP {
				anyErrors = append(anyErrors, err)
			}
			lg.With("handler", handlerName).Debug(
				fmt.Sprintf("opni alering request : %s and response %s", opniAlertingRequest, resp),
			)
		}
		if len(anyErrors) != 0 {
			for _, err := range anyErrors {
				if status.Code(err) != codes.NotFound {
					c.Status(http.StatusBadRequest)
					return
				} else { // return not found only if there are no other failed triggers
					c.Status(http.StatusNotFound)
				}
			}
			return
		}
		c.JSON(http.StatusOK, nil)
	})

	metricsRouter := gin.New()
	metricsRouter.GET("/healthz", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	if cfg.Profiling.Path != "" {
		pprof.Register(metricsRouter, cfg.Profiling.Path)
	} else {
		pprof.Register(metricsRouter)
	}

	metricsHandler := NewMetricsEndpointHandler(cfg.Metrics)
	metricsRouter.GET(cfg.Metrics.GetPath(), gin.WrapH(metricsHandler.Handler()))

	tlsConfig, _, err := loadTLSConfig(cfg)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to load serving cert bundle")
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
	pl.Hook(hooks.OnLoad(func(p types.MetricsPlugin) {
		srv.metricsRegisterer.MustRegister(p)
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

	metricsListener, err := net.Listen("tcp4", s.conf.MetricsListenAddress)

	lg.With(
		"api", listener.Addr().String(),
		"metrics", metricsListener.Addr().String(),
	).Info("gateway HTTP server starting")

	ctx, ca := context.WithCancel(ctx)

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
