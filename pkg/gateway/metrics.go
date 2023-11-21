package gateway

import (
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

func (h *MetricsEndpointHandler) Handler() http.Handler {
	return promhttp.HandlerFor(h.reg, promhttp.HandlerOpts{
		Registry: h.reg,
	})
}
