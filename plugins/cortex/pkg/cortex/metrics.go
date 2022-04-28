package cortex

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/opni/pkg/metrics/collector"
)

var (
	collectorServer  = collector.NewCollectorServer()
	ingestBytesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "opni",
		Subsystem: "gateway",
		Name:      "remote_write_ingest_bytes_total",
		Help:      "Total number of (compressed) bytes received from remote write requests",
	})
	ingestBytesByID = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "opni",
		Subsystem: "gateway",
		Name:      "remote_write_cluster_ingest_bytes",
		Help:      "Total number of (compressed) bytes received from remote write requests by cluster ID",
	}, []string{"cluster_id"})
)

func init() {
	collectorServer.MustRegister(
		ingestBytesTotal,
		ingestBytesByID,
	)
}
