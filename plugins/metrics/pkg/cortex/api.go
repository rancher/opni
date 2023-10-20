package cortex

import (
	"context"
	"crypto/tls"
	"os"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"log/slog"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
	"github.com/rancher/opni/pkg/rbac"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/fwd"
	metricsutil "github.com/rancher/opni/plugins/metrics/pkg/util"
)

type forwarders struct {
	QueryFrontend gin.HandlerFunc
	Alertmanager  gin.HandlerFunc
	Ruler         gin.HandlerFunc
}

type middlewares struct {
	RBAC gin.HandlerFunc
	Auth gin.HandlerFunc
}

type HttpApiServer struct {
	HttpApiServerConfig
	util.Initializer
}

type HttpApiServerConfig struct {
	PluginContext    context.Context               `validate:"required"`
	ManagementClient managementv1.ManagementClient `validate:"required"`
	CortexClientSet  ClientSet                     `validate:"required"`
	Config           *v1beta1.GatewayConfigSpec    `validate:"required"`
	CortexTLSConfig  *tls.Config                   `validate:"required"`
	Logger           *slog.Logger                  `validate:"required"`
	StorageBackend   storage.Backend               `validate:"required"`
	AuthMiddlewares  map[string]auth.Middleware    `validate:"required"`
}

func (p *HttpApiServer) Initialize(config HttpApiServerConfig) {
	p.InitOnce(func() {
		if err := metricsutil.Validate.Struct(config); err != nil {
			panic(err)
		}
		p.HttpApiServerConfig = config
	})
}

var _ httpext.HTTPAPIExtension = (*HttpApiServer)(nil)

func (p *HttpApiServer) ConfigureRoutes(router *gin.Engine) {
	p.WaitForInit()

	p.Logger.Info("configuring http api server")

	router.Use(logger.GinLogger(p.Logger), gin.Recovery())

	rbacProvider := storage.NewRBACProvider(p.StorageBackend)
	rbacMiddleware := rbac.NewMiddleware(rbacProvider, orgIDCodec)
	authMiddleware, ok := p.AuthMiddlewares[p.Config.AuthProvider]
	if !ok {
		p.Logger.With(
			"name", p.Config.AuthProvider,
		).Error("auth provider not found")
		os.Exit(1)
	}

	fwds := &forwarders{
		QueryFrontend: fwd.To(p.Config.Cortex.QueryFrontend.HTTPAddress, fwd.WithLogger(p.Logger), fwd.WithTLS(p.CortexTLSConfig), fwd.WithName("query-frontend")),
		Alertmanager:  fwd.To(p.Config.Cortex.Alertmanager.HTTPAddress, fwd.WithLogger(p.Logger), fwd.WithTLS(p.CortexTLSConfig), fwd.WithName("alertmanager")),
		Ruler:         fwd.To(p.Config.Cortex.Ruler.HTTPAddress, fwd.WithLogger(p.Logger), fwd.WithTLS(p.CortexTLSConfig), fwd.WithName("ruler")),
	}

	mws := &middlewares{
		RBAC: rbacMiddleware,
		Auth: authMiddleware.(auth.HTTPMiddleware).Handle,
	}

	router.GET("/ready", fwds.QueryFrontend)

	// p.configureAgentAPI(app, fwds, mws)
	p.configureAlertmanager(router, fwds, mws)
	p.configureRuler(router, fwds, mws)
	p.configureQueryFrontend(router, fwds, mws)
	pprof.Register(router, "/debug/plugin_metrics/pprof")
}

func (p *HttpApiServer) configureAlertmanager(router *gin.Engine, f *forwarders, m *middlewares) {
	orgIdLimiter := func(c *gin.Context) {
		ids := rbac.AuthorizedClusterIDs(c)
		if len(ids) > 1 {
			user, _ := rbac.AuthorizedUserID(c)
			p.Logger.With(
				"request", c.FullPath(),
				"user", user,
			).Debug("multiple org ids found, limiting to first")
			c.Header(orgIDCodec.Key(), orgIDCodec.Encode(ids[:1]))
		}
		return
	}
	router.Any("/api/prom/alertmanager", m.Auth, m.RBAC, orgIdLimiter, f.Alertmanager)
	router.Any("/api/v1/alerts", m.Auth, m.RBAC, orgIdLimiter, f.Alertmanager)
	router.Any("/multitenant_alertmanager", m.Auth, m.RBAC, orgIdLimiter, f.Alertmanager)
}

func (p *HttpApiServer) configureRuler(router *gin.Engine, f *forwarders, m *middlewares) {
	jsonAggregator := NewMultiTenantRuleAggregator(
		p.ManagementClient, p.CortexClientSet.HTTP(), orgIDCodec, PrometheusRuleGroupsJSON)
	router.GET("/prometheus/api/v1/rules", m.Auth, m.RBAC, jsonAggregator.Handle)
	router.GET("/api/prom/api/v1/rules", m.Auth, m.RBAC, jsonAggregator.Handle)

	router.GET("/prometheus/api/v1/alerts", m.Auth, m.RBAC, f.Ruler)
	router.GET("/api/prom/api/v1/alerts", m.Auth, m.RBAC, f.Ruler)

	yamlAggregator := NewMultiTenantRuleAggregator(
		p.ManagementClient, p.CortexClientSet.HTTP(), orgIDCodec, NamespaceKeyedYAML)
	router.Any("/api/v1/rules", m.Auth, m.RBAC, yamlAggregator.Handle)
	router.Any("/api/prom/rules", m.Auth, m.RBAC, yamlAggregator.Handle)
}

func (p *HttpApiServer) configureQueryFrontend(router *gin.Engine, f *forwarders, m *middlewares) {
	for _, group := range []*gin.RouterGroup{
		router.Group("/prometheus/api/v1", m.Auth, m.RBAC),
		router.Group("/api/prom/api/v1", m.Auth, m.RBAC),
	} {
		group.POST("/read", f.QueryFrontend)
		group.GET("/query", f.QueryFrontend)
		group.POST("/query", f.QueryFrontend)
		group.GET("/query_range", f.QueryFrontend)
		group.POST("/query_range", f.QueryFrontend)
		group.GET("/query_exemplars", f.QueryFrontend)
		group.POST("/query_exemplars", f.QueryFrontend)
		group.GET("/labels", f.QueryFrontend)
		group.POST("/labels", f.QueryFrontend)
		group.GET("/label/:name/values", f.QueryFrontend)
		group.GET("/series", f.QueryFrontend)
		group.POST("/series", f.QueryFrontend)
		group.DELETE("/series", f.QueryFrontend)
		group.GET("/metadata", f.QueryFrontend)
	}
}
