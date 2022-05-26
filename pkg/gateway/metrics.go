package gateway

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rancher/opni/pkg/config/v1beta1"
)

type MetricsEndpointHandler struct {
	reg *prometheus.Registry
	cfg v1beta1.MetricsSpec
}

func NewMetricsEndpointHandler(cfg v1beta1.MetricsSpec) *MetricsEndpointHandler {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	reg.MustRegister(collectors.NewBuildInfoCollector())
	reg.MustRegister(collectors.NewGoCollector())
	return &MetricsEndpointHandler{
		reg: reg,
		cfg: cfg,
	}
}

func (h *MetricsEndpointHandler) ListenAndServe(ctx context.Context) error {
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp4", fmt.Sprintf(":%d", h.cfg.Port))
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle(h.cfg.GetPath(), promhttp.HandlerFor(h.reg, promhttp.HandlerOpts{
		Registry: h.reg,
	}))
	return http.Serve(listener, mux)
}

func (h *MetricsEndpointHandler) MustRegister(collectors ...prometheus.Collector) {
	h.reg.MustRegister(collectors...)
}
