package cortex

import (
	"context"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rancher/opni/pkg/auth"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/rbac"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util/fwd"
)

type forwarders struct {
	QueryFrontend fiber.Handler
	Alertmanager  fiber.Handler
	Ruler         fiber.Handler
	Distributor   fiber.Handler
}

type middlewares struct {
	RBAC    fiber.Handler
	Auth    fiber.Handler
	Cluster fiber.Handler
}

func (p *Plugin) ConfigureRoutes(app *fiber.App) {
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
		QueryFrontend: fwd.To(config.Spec.Cortex.QueryFrontend.HTTPAddress, fwd.WithTLS(cortexTLSConfig), fwd.WithName("cortex.query-frontend")),
		Alertmanager:  fwd.To(config.Spec.Cortex.Alertmanager.HTTPAddress, fwd.WithTLS(cortexTLSConfig), fwd.WithName("cortex.alertmanager")),
		Ruler:         fwd.To(config.Spec.Cortex.Ruler.HTTPAddress, fwd.WithTLS(cortexTLSConfig), fwd.WithName("cortex.ruler")),
		Distributor:   fwd.To(config.Spec.Cortex.Distributor.HTTPAddress, fwd.WithTLS(cortexTLSConfig), fwd.WithName("cortex.distributor")),
	}

	mws := &middlewares{
		RBAC:    rbacMiddleware,
		Auth:    authMiddleware.(auth.HTTPMiddleware).Handle,
		Cluster: clusterMiddleware.Handle,
	}

	app.Get("/ready", fwds.QueryFrontend)

	// p.configureAgentAPI(app, fwds, mws)
	p.configureAlertmanager(app, fwds, mws)
	p.configureRuler(app, fwds, mws)
	p.configureQueryFrontend(app, fwds, mws)
}

func (p *Plugin) preprocessRules(c *fiber.Ctx) error {
	id := cluster.AuthorizedID(c)
	p.logger.With(
		"id", id,
	).Info("syncing cluster alert rules")
	c.Path("/api/v1/rules/" + id)
	return c.Next()
}

func (p *Plugin) configureAlertmanager(app *fiber.App, f *forwarders, m *middlewares) {
	orgIdLimiter := func(c *fiber.Ctx) error {
		ids := rbac.AuthorizedClusterIDs(c)
		if len(ids) > 1 {
			user, _ := rbac.AuthorizedUserID(c)
			p.logger.With(
				"request", c.Path(),
				"user", user,
			).Debug("multiple org ids found, limiting to first")
			c.Request().Header.Set(orgIDCodec.Key(), orgIDCodec.Encode(ids[:1]))
		}
		return c.Next()
	}
	app.Use("/api/prom/alertmanager", m.Auth, m.RBAC, orgIdLimiter, f.Alertmanager)

	app.Use("/api/v1/alerts", m.Auth, m.RBAC, orgIdLimiter, f.Alertmanager)
	app.Use("/api/prom/api/v1/alerts", func(c *fiber.Ctx) error {
		c.Path("/api/v1/alerts")
		return c.Next()
	}, m.Auth, m.RBAC, orgIdLimiter, f.Alertmanager)

	app.Use("/multitenant_alertmanager", m.Auth, m.RBAC, orgIdLimiter, f.Alertmanager)
}

func (p *Plugin) configureRuler(app *fiber.App, f *forwarders, m *middlewares) {
	jsonAggregator := NewMultiTenantRuleAggregator(
		p.mgmtApi.Get(), f.Ruler, orgIDCodec, PrometheusRuleGroupsJSON)
	app.Get("/prometheus/api/v1/rules", m.Auth, m.RBAC, jsonAggregator.Handle)
	app.Get("/api/prom/api/v1/rules", m.Auth, m.RBAC, jsonAggregator.Handle)

	app.Get("/prometheus/api/v1/alerts", m.Auth, m.RBAC, f.Ruler)
	app.Get("/api/prom/api/v1/alerts", m.Auth, m.RBAC, f.Ruler)

	yamlAggregator := NewMultiTenantRuleAggregator(
		p.mgmtApi.Get(), f.Ruler, orgIDCodec, NamespaceKeyedYAML)
	app.Use("/api/v1/rules", m.Auth, m.RBAC, yamlAggregator.Handle)
	app.Use("/api/prom/rules", m.Auth, m.RBAC, yamlAggregator.Handle)
}

func (p *Plugin) configureQueryFrontend(app *fiber.App, f *forwarders, m *middlewares) {
	for _, group := range []fiber.Router{
		app.Group("/prometheus/api/v1", m.Auth, m.RBAC),
		app.Group("/api/prom/api/v1", m.Auth, m.RBAC),
	} {
		group.Post("/read", f.QueryFrontend)
		group.Get("/query", f.QueryFrontend)
		group.Post("/query", f.QueryFrontend)
		group.Get("/query_range", f.QueryFrontend)
		group.Post("/query_range", f.QueryFrontend)
		group.Get("/query_exemplars", f.QueryFrontend)
		group.Post("/query_exemplars", f.QueryFrontend)
		group.Get("/labels", f.QueryFrontend)
		group.Post("/labels", f.QueryFrontend)
		group.Get("/label/:name/values", f.QueryFrontend)
		group.Get("/series", f.QueryFrontend)
		group.Post("/series", f.QueryFrontend)
		group.Delete("/series", f.QueryFrontend)
		group.Get("/metadata", f.QueryFrontend)
	}
}
