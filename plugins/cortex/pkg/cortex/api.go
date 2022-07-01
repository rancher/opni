package cortex

import (
	"context"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/rbac"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util/fwd"
)

type forwarders struct {
	QueryFrontend gin.HandlerFunc
	Alertmanager  gin.HandlerFunc
	Ruler         gin.HandlerFunc
}

type middlewares struct {
	RBAC    gin.HandlerFunc
	Auth    gin.HandlerFunc
	Cluster gin.HandlerFunc
}

func (p *Plugin) ConfigureRoutes(router *gin.Engine) {
	router.Use(logger.GinLogger(p.logger), gin.Recovery())

	futureCtx, ca := context.WithTimeout(context.Background(), 10*time.Second)
	defer ca()
	config, err := p.config.GetContext(futureCtx)
	if err != nil {
		p.logger.With("err", err).Error("plugin startup failed: config was not loaded")
		os.Exit(1)
	}

	futureCtx, ca = context.WithTimeout(context.Background(), 10*time.Second)
	defer ca()
	cortexTLSConfig, err := p.cortexTlsConfig.GetContext(futureCtx)
	if err != nil {
		p.logger.With("err", err).Error("plugin startup failed: cortex TLS config was not loaded")
		os.Exit(1)
	}

	futureCtx, ca = context.WithTimeout(context.Background(), 10*time.Second)
	defer ca()
	storageBackend, err := p.storageBackend.GetContext(futureCtx)
	if err != nil {
		p.logger.With("err", err).Error("plugin startup failed: storage backend was not loaded")
		os.Exit(1)
	}

	rbacProvider := storage.NewRBACProvider(storageBackend)
	rbacMiddleware := rbac.NewMiddleware(rbacProvider, orgIDCodec)
	authMiddleware, ok := p.authMiddlewares.Get()[config.Spec.AuthProvider]
	if !ok {
		p.logger.With(
			"name", config.Spec.AuthProvider,
		).Error("auth provider not found")
		os.Exit(1)
	}
	clusterMiddleware, err := cluster.New(p.ctx, storageBackend, orgIDCodec.Key())
	if err != nil {
		p.logger.With(
			"err", err,
		).Error("failed to set up cluster middleware")
		os.Exit(1)
	}

	fwds := &forwarders{
		QueryFrontend: fwd.To(config.Spec.Cortex.QueryFrontend.HTTPAddress, fwd.WithLogger(p.logger), fwd.WithTLS(cortexTLSConfig), fwd.WithName("query-frontend")),
		Alertmanager:  fwd.To(config.Spec.Cortex.Alertmanager.HTTPAddress, fwd.WithLogger(p.logger), fwd.WithTLS(cortexTLSConfig), fwd.WithName("alertmanager")),
		Ruler:         fwd.To(config.Spec.Cortex.Ruler.HTTPAddress, fwd.WithLogger(p.logger), fwd.WithTLS(cortexTLSConfig), fwd.WithName("ruler")),
	}

	mws := &middlewares{
		RBAC:    rbacMiddleware,
		Auth:    authMiddleware.(auth.HTTPMiddleware).Handle,
		Cluster: clusterMiddleware.Handle,
	}

	router.GET("/ready", fwds.QueryFrontend)

	// p.configureAgentAPI(app, fwds, mws)
	p.configureAlertmanager(router, fwds, mws)
	p.configureRuler(router, fwds, mws)
	p.configureQueryFrontend(router, fwds, mws)
}

func (p *Plugin) configureAlertmanager(router *gin.Engine, f *forwarders, m *middlewares) {
	orgIdLimiter := func(c *gin.Context) {
		ids := rbac.AuthorizedClusterIDs(c)
		if len(ids) > 1 {
			user, _ := rbac.AuthorizedUserID(c)
			p.logger.With(
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

func (p *Plugin) configureRuler(router *gin.Engine, f *forwarders, m *middlewares) {
	jsonAggregator := NewMultiTenantRuleAggregator(
		p.mgmtApi.Get(), p.cortexHttpClient.Get(), orgIDCodec, PrometheusRuleGroupsJSON)
	router.GET("/prometheus/api/v1/rules", m.Auth, m.RBAC, jsonAggregator.Handle)
	router.GET("/api/prom/api/v1/rules", m.Auth, m.RBAC, jsonAggregator.Handle)

	router.GET("/prometheus/api/v1/alerts", m.Auth, m.RBAC, f.Ruler)
	router.GET("/api/prom/api/v1/alerts", m.Auth, m.RBAC, f.Ruler)

	yamlAggregator := NewMultiTenantRuleAggregator(
		p.mgmtApi.Get(), p.cortexHttpClient.Get(), orgIDCodec, NamespaceKeyedYAML)
	router.Any("/api/v1/rules", m.Auth, m.RBAC, yamlAggregator.Handle)
	router.Any("/api/prom/rules", m.Auth, m.RBAC, yamlAggregator.Handle)
}

func (p *Plugin) configureQueryFrontend(router *gin.Engine, f *forwarders, m *middlewares) {
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
