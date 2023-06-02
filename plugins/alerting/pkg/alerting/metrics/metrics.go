package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	SyncCycleProcessLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "opni",
			Subsystem: "alerting",
			Name:      "sync_cycle_processing_latency_ms",
			Help:      "Latency of alerting sync cycles in milliseconds",
			Buckets:   []float64{10, 25, 35, 50, 65, 80, 100},
		},
	)

	SyncCycleFailedCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "opni",
			Subsystem: "alerting",
			Name:      "sync_cycle_failure_count",
			Help:      "Number of alerting sync cycle failures",
		},
	)

	SyncCycleCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "opni",
			Subsystem: "alerting",
			Name:      "sync_cycle_total_count",
			Help:      "Total number of alerting sync cycles executed",
		},
	)

	AlarmActivationFailureCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "opni",
			Subsystem: "alerting",
			Name:      "alarm_activation_failure_count",
			Help:      "Total number of alerting alarm activation failures",
		}, []string{"cluster_id", "alarm_name", "datasource", "alarm_id"},
	)

	AlarmActivationCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "opni",
			Subsystem: "alerting",
			Name:      "alarm_activation_count",
			Help:      "Total number of alerting alarm activations",
		}, []string{"cluster_id", "alarm_name", "datasource", "alarm_id"},
	)
)
