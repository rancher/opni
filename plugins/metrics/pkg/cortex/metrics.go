package cortex

import (
	"sync"

	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/aggregation"
)

var (
	muMeterProvider                  sync.Mutex
	cIngestBytesByID                 metric.Int64Counter
	cRemoteWriteRequests             metric.Int64Counter
	hRemoteWriteProcessingLatency    metric.Int64Histogram
	cRemoteWriteTotalProcessedSeries metric.Int64Counter
	meterProvider                    *sdkmetric.MeterProvider
)

func RegisterMeterProvider(mp *sdkmetric.MeterProvider) {
	muMeterProvider.Lock()
	defer muMeterProvider.Unlock()
	meterProvider = mp
	createMetrics()
}

func createMetrics() {
	meter := meterProvider.Meter("gateway")
	var err error
	cIngestBytesByID, err = meter.Int64Counter("remote_write_cluster_ingest_bytes",
		metric.WithDescription("Total number of (compressed) bytes received from remote write requests by cluster ID"))
	if err != nil {
		panic(err)
	}
	cRemoteWriteRequests, err = meter.Int64Counter("remote_write_requests_total",
		metric.WithDescription("Total number of remote write requests forwarded to Cortex"))
	if err != nil {
		panic(err)
	}
	hRemoteWriteProcessingLatency, err = meter.Int64Histogram("remote_write_processing_latency_ns",
		metric.WithDescription("Latency of remote write processing in nanoseconds per timeseries"),
		metric.WithUnit("ns"),
	)
	if err != nil {
		panic(err)
	}
	cRemoteWriteTotalProcessedSeries, err = meter.Int64Counter("remote_write_total_processed_series",
		metric.WithDescription("Total number of series processed by remote write"))
	if err != nil {
		panic(err)
	}
}

func CortexAggregationSelector(ik sdkmetric.InstrumentKind) aggregation.Aggregation {
	switch ik {
	case sdkmetric.InstrumentKindCounter, sdkmetric.InstrumentKindUpDownCounter,
		sdkmetric.InstrumentKindObservableCounter, sdkmetric.InstrumentKindObservableUpDownCounter:
		return aggregation.Sum{}
	case sdkmetric.InstrumentKindObservableGauge:
		return aggregation.LastValue{}
	case sdkmetric.InstrumentKindHistogram:
		return aggregation.ExplicitBucketHistogram{
			Boundaries: []float64{30, 35, 37.5, 40, 42.5, 45, 50, 55, 60, 75, 100},
			NoMinMax:   false,
		}
	}
	panic("unknown instrument kind")
}

func init() {
	muMeterProvider.Lock()
	defer muMeterProvider.Unlock()
	if meterProvider == nil {
		meterProvider = sdkmetric.NewMeterProvider()
	}
	createMetrics()
}
