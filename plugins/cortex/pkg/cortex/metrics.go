package cortex

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/opni-monitoring/pkg/metrics/collector"
)

var (
	collectorServer = collector.NewCollectorServer()
)

func init() {
	collectorServer.MustRegister(prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "opni",
		Subsystem: "gateway",
		Name:      "remote_write_ingest_bytes_total",
		Help:      "Total number of bytes received from remote write requests",
	}))
}
