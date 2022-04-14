package gateway

import (
	"net"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsEndpointHandler struct {
	reg *prometheus.Registry
}

func NewMetricsEndpointHandler() *MetricsEndpointHandler {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	reg.MustRegister(collectors.NewBuildInfoCollector())
	reg.MustRegister(collectors.NewGoCollector())
	return &MetricsEndpointHandler{
		reg: reg,
	}
}

func (h *MetricsEndpointHandler) ListenAndServe(listener net.Listener) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(h.reg, promhttp.HandlerOpts{
		Registry: h.reg,
	}))
	return http.Serve(listener, mux)
}

func (h *MetricsEndpointHandler) MustRegister(collectors ...prometheus.Collector) {
	h.reg.MustRegister(collectors...)
}
