package cortex

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	mIngestBytesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "opni",
		Subsystem: "gateway",
		Name:      "remote_write_ingest_bytes_total",
		Help:      "Total number of (compressed) bytes received from remote write requests",
	})
	mIngestBytesByID = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "opni",
		Subsystem: "gateway",
		Name:      "remote_write_cluster_ingest_bytes",
		Help:      "Total number of (compressed) bytes received from remote write requests by cluster ID",
	}, []string{"cluster_id"})
	mRemoteWriteRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "opni",
		Subsystem: "gateway",
		Name:      "remote_write_requests_total",
		Help:      "Total number of remote write requests forwarded to Cortex",
	}, []string{"cluster_id", "code", "code_text"})
	mRemoteWriteProcessingLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "opni",
		Subsystem: "gateway",
		Name:      "remote_write_processing_latency_ns",
		Help:      "Latency of remote write processing in nanoseconds per timeseries",
		Buckets:   prometheus.ExponentialBuckets(1, 2, 10),
	})
	mRemoteWriteTotalProcessedSeries = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "opni",
		Subsystem: "gateway",
		Name:      "remote_write_total_processed_series",
		Help:      "Total number of series processed by remote write",
	})
)

func Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		mIngestBytesTotal,
		mIngestBytesByID,
		mRemoteWriteRequests,
		mRemoteWriteProcessingLatency,
		mRemoteWriteTotalProcessedSeries,
	}
}
