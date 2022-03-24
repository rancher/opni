package cortex

import (
	"os"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/rancher/opni-monitoring/pkg/auth"
	"github.com/rancher/opni-monitoring/pkg/auth/cluster"
	"github.com/rancher/opni-monitoring/pkg/rbac"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/util/fwd"
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
	config := p.config.Get()

	cortexTLSConfig := p.loadCortexCerts()

	storageBackend := p.storageBackend.Get()
	rbacProvider := storage.NewRBACProvider(storageBackend)
	rbacMiddleware := rbac.NewMiddleware(rbacProvider, orgIDCodec)
	authMiddleware, err := auth.GetMiddleware(config.Spec.AuthProvider)
	if err != nil {
		p.logger.With(
			"err", err,
		).Error("failed to get auth middleware")
		os.Exit(1)
	}
	clusterMiddleware := cluster.New(storageBackend, orgIDCodec.Key())

	fwds := &forwarders{
		QueryFrontend: fwd.To(config.Spec.Cortex.QueryFrontend.HTTPAddress, fwd.WithTLS(cortexTLSConfig), fwd.WithName("cortex.query-frontend")),
		Alertmanager:  fwd.To(config.Spec.Cortex.Alertmanager.HTTPAddress, fwd.WithTLS(cortexTLSConfig), fwd.WithName("cortex.alertmanager")),
		Ruler:         fwd.To(config.Spec.Cortex.Ruler.HTTPAddress, fwd.WithTLS(cortexTLSConfig), fwd.WithName("cortex.ruler")),
		Distributor:   fwd.To(config.Spec.Cortex.Distributor.HTTPAddress, fwd.WithTLS(cortexTLSConfig), fwd.WithName("cortex.distributor")),
	}

	mws := &middlewares{
		RBAC:    rbacMiddleware,
		Auth:    authMiddleware.Handle,
		Cluster: clusterMiddleware.Handle,
	}

	app.Get("/ready", fwds.QueryFrontend)

	p.configureAgentAPI(app, fwds, mws)
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

func (p *Plugin) configureAgentAPI(app *fiber.App, f *forwarders, m *middlewares) {
	g := app.Group("/api/agent", m.Cluster)
	g.Post("/push", func(c *fiber.Ctx) error {
		clusterID := cluster.AuthorizedID(c)
		len := c.Get("Content-Length", "0")
		if i, err := strconv.ParseInt(len, 10, 64); err == nil && i > 0 {
			flen := float64(i)
			ingestBytesTotal.Add(flen)
			ingestBytesByID.With(map[string]string{
				"cluster_id": clusterID,
			}).Add(flen)
		}
		c.Path("/api/v1/push")
		return c.Next()
	}, f.Distributor)
	g.Post("/sync_rules", p.preprocessRules, f.Ruler)
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
