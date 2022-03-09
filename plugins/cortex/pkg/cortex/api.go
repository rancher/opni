package cortex

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/rancher/opni-monitoring/pkg/auth"
	"github.com/rancher/opni-monitoring/pkg/auth/cluster"
	"github.com/rancher/opni-monitoring/pkg/rbac"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/util/fwd"
)

func (p *Plugin) ConfigureRoutes(app *fiber.App) {
	config := p.config.Get()

	cortexTLSConfig := p.loadCortexCerts()

	queryFrontend := fwd.To(config.Spec.Cortex.QueryFrontend.Address, fwd.WithTLS(cortexTLSConfig), fwd.WithName("cortex.query-frontend"))
	alertmanager := fwd.To(config.Spec.Cortex.Alertmanager.Address, fwd.WithTLS(cortexTLSConfig), fwd.WithName("cortex.alertmanager"))
	ruler := fwd.To(config.Spec.Cortex.Ruler.Address, fwd.WithTLS(cortexTLSConfig), fwd.WithName("cortex.ruler"))
	distributor := fwd.To(config.Spec.Cortex.Distributor.Address, fwd.WithTLS(cortexTLSConfig), fwd.WithName("cortex.distributor"))

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

	app.Get("/services", queryFrontend)
	app.Get("/ready", queryFrontend)

	// Memberlist
	app.Get("/ring", distributor)
	app.Get("/ruler/ring", ruler)

	// Alertmanager UI
	alertmanagerUi := app.Group("/alertmanager", authMiddleware.Handle, rbacMiddleware)
	alertmanagerUi.Get("/alertmanager", alertmanager)

	// Prometheus-compatible API
	promv1 := app.Group("/prometheus/api/v1", authMiddleware.Handle, rbacMiddleware)

	// GET, POST
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		promv1.Add(method, "/query", queryFrontend)
		promv1.Add(method, "/query_range", queryFrontend)
		promv1.Add(method, "/query_exemplars", queryFrontend)
		promv1.Add(method, "/series", queryFrontend)
		promv1.Add(method, "/labels", queryFrontend)
	}
	// GET only
	promv1.Get("/rules", ruler)
	promv1.Get("/alerts", ruler)
	promv1.Get("/label/:name/values", queryFrontend)
	promv1.Get("/metadata", queryFrontend)

	// POST only
	promv1.Post("/read", queryFrontend)

	// Remote-write API
	clusterMiddleware := cluster.New(storageBackend, orgIDCodec.Key())
	v1 := app.Group("/api/v1", clusterMiddleware.Handle)
	v1.Post("/push", distributor)
}

func (p *Plugin) loadCortexCerts() *tls.Config {
	lg := p.logger
	config := p.config.Get()

	cortexServerCA := config.Spec.Cortex.Certs.ServerCA
	cortexClientCA := config.Spec.Cortex.Certs.ClientCA
	cortexClientCert := config.Spec.Cortex.Certs.ClientCert
	cortexClientKey := config.Spec.Cortex.Certs.ClientKey

	clientCert, err := tls.LoadX509KeyPair(cortexClientCert, cortexClientKey)
	if err != nil {
		lg.With(
			"err", err,
		).Error("fatal: failed to load cortex client keypair")
		os.Exit(1)
	}
	serverCAPool := x509.NewCertPool()
	serverCAData, err := os.ReadFile(cortexServerCA)
	if err != nil {
		lg.With(
			"err", err,
		).Error("fatal: failed to read cortex server CA")
		os.Exit(1)
	}
	if ok := serverCAPool.AppendCertsFromPEM(serverCAData); !ok {
		lg.Error("fatal: failed to load cortex server CA")
		os.Exit(1)
	}
	clientCAPool := x509.NewCertPool()
	clientCAData, err := os.ReadFile(cortexClientCA)
	if err != nil {
		lg.With(
			"err", err,
		).Error("fatal: failed to read cortex client CA")
		os.Exit(1)
	}
	if ok := clientCAPool.AppendCertsFromPEM(clientCAData); !ok {
		lg.Error("fatal: failed to load cortex client CA")
		os.Exit(1)
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{clientCert},
		ClientCAs:    clientCAPool,
		RootCAs:      serverCAPool,
	}
}
