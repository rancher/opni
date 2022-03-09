package gateway

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"

	"github.com/rancher/opni-monitoring/pkg/auth/cluster"
	"github.com/rancher/opni-monitoring/pkg/rbac"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/util/fwd"
	"go.uber.org/zap"
)

// *****************************************************************************
// * NOTE: all logic here will be moved into a plugin - this code is temporary *
// *****************************************************************************

func (g *Gateway) loadCortexCerts() {
	lg := g.logger
	cortexServerCA := g.config.Spec.Cortex.Certs.ServerCA
	cortexClientCA := g.config.Spec.Cortex.Certs.ClientCA
	cortexClientCert := g.config.Spec.Cortex.Certs.ClientCert
	cortexClientKey := g.config.Spec.Cortex.Certs.ClientKey

	clientCert, err := tls.LoadX509KeyPair(cortexClientCert, cortexClientKey)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatal("failed to load cortex client keypair")
	}
	serverCAPool := x509.NewCertPool()
	serverCAData, err := os.ReadFile(cortexServerCA)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatalf("failed to read cortex server CA")
	}
	if ok := serverCAPool.AppendCertsFromPEM(serverCAData); !ok {
		lg.Fatal("failed to load cortex server CA")
	}
	clientCAPool := x509.NewCertPool()
	clientCAData, err := os.ReadFile(cortexClientCA)
	if err != nil {
		lg.With(
			zap.Error(err),
		).Fatalf("failed to read cortex client CA")
	}
	if ok := clientCAPool.AppendCertsFromPEM(clientCAData); !ok {
		lg.Fatal("failed to load cortex client CA")
	}
	g.cortexTLSConfig = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{clientCert},
		ClientCAs:    clientCAPool,
		RootCAs:      serverCAPool,
	}
}

func (g *Gateway) setupCortexRoutes(
	rbacStore storage.RBACStore,
	clusterStore storage.ClusterStore,
) {
	queryFrontend := fwd.To(g.config.Spec.Cortex.QueryFrontend.Address, fwd.WithTLS(g.cortexTLSConfig), fwd.WithName("cortex.query-frontend"))
	alertmanager := fwd.To(g.config.Spec.Cortex.Alertmanager.Address, fwd.WithTLS(g.cortexTLSConfig), fwd.WithName("cortex.alertmanager"))
	ruler := fwd.To(g.config.Spec.Cortex.Ruler.Address, fwd.WithTLS(g.cortexTLSConfig), fwd.WithName("cortex.ruler"))
	distributor := fwd.To(g.config.Spec.Cortex.Distributor.Address, fwd.WithTLS(g.cortexTLSConfig), fwd.WithName("cortex.distributor"))

	rbacProvider := storage.NewRBACProvider(g.storageBackend)
	rbacMiddleware := rbac.NewMiddleware(rbacProvider)

	g.apiServer.app.Get("/services", queryFrontend)
	g.apiServer.app.Get("/ready", queryFrontend)

	// Memberlist
	g.apiServer.app.Get("/ring", distributor)
	g.apiServer.app.Get("/ruler/ring", ruler)

	// Alertmanager UI
	alertmanagerUi := g.apiServer.app.Group("/alertmanager", g.apiServer.authMiddleware.Handle, rbacMiddleware)
	alertmanagerUi.Get("/alertmanager", alertmanager)

	// Prometheus-compatible API
	promv1 := g.apiServer.app.Group("/prometheus/api/v1", g.apiServer.authMiddleware.Handle, rbacMiddleware)

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
	clusterMiddleware := cluster.New(clusterStore)
	v1 := g.apiServer.app.Group("/api/v1", clusterMiddleware.Handle)
	v1.Post("/push", distributor)
}
