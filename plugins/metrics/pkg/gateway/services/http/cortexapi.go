package http

import (
	"os"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"

	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/logger"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/rbac"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util/fwd"
	"github.com/rancher/opni/plugins/metrics/pkg/cortex"
	"github.com/rancher/opni/plugins/metrics/pkg/types"
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

type CortexApiService struct {
	sc types.ServiceContext

	cortexClientSet cortex.ClientSet
}

func (p *CortexApiService) Activate(ctx types.ServiceContext) error {
	p.sc = ctx
	clientSet, err := ctx.Memoize(cortex.ClientSetKey, func() (any, error) {
		tlsConfig, err := cortex.LoadTLSConfig(ctx.GatewayConfig())
		if err != nil {
			return nil, err
		}
		return cortex.NewClientSet(ctx, &ctx.GatewayConfig().Spec.Cortex, tlsConfig)
	})
	if err != nil {
		return err
	}
	p.cortexClientSet = clientSet.(cortex.ClientSet)
	return nil
}

func (p *CortexApiService) AddToScheme(scheme meta.Scheme) {
	scheme.Add(httpext.HTTPAPIExtensionPluginID, httpext.NewPlugin(p))
}

var _ httpext.HTTPAPIExtension = (*CortexApiService)(nil)

func (p *CortexApiService) ConfigureRoutes(router *gin.Engine) {
	lg := p.sc.Logger()
	lg.Info("configuring http api server")

	router.Use(logger.GinLogger(lg), gin.Recovery())
	config := p.sc.GatewayConfig().Spec

	rbacProvider := storage.NewRBACProvider(p.sc.StorageBackend())
	rbacMiddleware := rbac.NewMiddleware(rbacProvider, cortex.OrgIDCodec)
	authMiddleware, ok := p.sc.AuthMiddlewares()[config.AuthProvider]
	if !ok {
		lg.With(
			"name", config.AuthProvider,
		).Error("auth provider not found")
		os.Exit(1)
	}

	fwds := &forwarders{
		QueryFrontend: fwd.To(config.Cortex.QueryFrontend.HTTPAddress, fwd.WithLogger(lg), fwd.WithTLS(p.cortexClientSet.TLSConfig()), fwd.WithName("query-frontend")),
		Alertmanager:  fwd.To(config.Cortex.Alertmanager.HTTPAddress, fwd.WithLogger(lg), fwd.WithTLS(p.cortexClientSet.TLSConfig()), fwd.WithName("alertmanager")),
		Ruler:         fwd.To(config.Cortex.Ruler.HTTPAddress, fwd.WithLogger(lg), fwd.WithTLS(p.cortexClientSet.TLSConfig()), fwd.WithName("ruler")),
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

func (p *CortexApiService) configureAlertmanager(router *gin.Engine, f *forwarders, m *middlewares) {
	orgIdLimiter := func(c *gin.Context) {
		ids := rbac.AuthorizedClusterIDs(c)
		if len(ids) > 1 {
			user, _ := rbac.AuthorizedUserID(c)
			p.sc.Logger().With(
				"request", c.FullPath(),
				"user", user,
			).Debug("multiple org ids found, limiting to first")
			c.Header(cortex.OrgIDCodec.Key(), cortex.OrgIDCodec.Encode(ids[:1]))
		}
		return
	}
	router.Any("/api/prom/alertmanager", m.Auth, m.RBAC, orgIdLimiter, f.Alertmanager)
	router.Any("/api/v1/alerts", m.Auth, m.RBAC, orgIdLimiter, f.Alertmanager)
	router.Any("/multitenant_alertmanager", m.Auth, m.RBAC, orgIdLimiter, f.Alertmanager)
}

func (p *CortexApiService) configureRuler(router *gin.Engine, f *forwarders, m *middlewares) {
	jsonAggregator := cortex.NewMultiTenantRuleAggregator(
		p.sc.ManagementClient(), p.cortexClientSet.HTTP(), cortex.OrgIDCodec, cortex.PrometheusRuleGroupsJSON)
	router.GET("/prometheus/api/v1/rules", m.Auth, m.RBAC, jsonAggregator.Handle)
	router.GET("/api/prom/api/v1/rules", m.Auth, m.RBAC, jsonAggregator.Handle)

	router.GET("/prometheus/api/v1/alerts", m.Auth, m.RBAC, f.Ruler)
	router.GET("/api/prom/api/v1/alerts", m.Auth, m.RBAC, f.Ruler)

	yamlAggregator := cortex.NewMultiTenantRuleAggregator(
		p.sc.ManagementClient(), p.cortexClientSet.HTTP(), cortex.OrgIDCodec, cortex.NamespaceKeyedYAML)
	router.Any("/api/v1/rules", m.Auth, m.RBAC, yamlAggregator.Handle)
	router.Any("/api/prom/rules", m.Auth, m.RBAC, yamlAggregator.Handle)
}

func (p *CortexApiService) configureQueryFrontend(router *gin.Engine, f *forwarders, m *middlewares) {
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
