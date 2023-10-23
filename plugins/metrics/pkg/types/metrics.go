package types

import (
	"github.com/rancher/opni/pkg/util"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/aggregation"
)

type Metrics struct {
	RemoteWriteProcessingLatency    metric.Int64Histogram
	RemoteWriteTotalProcessedSeries metric.Int64Counter
}

func NewMetrics(meterProvider *sdkmetric.MeterProvider) *Metrics {
	meter := meterProvider.Meter("gateway")
	return &Metrics{
		RemoteWriteProcessingLatency: util.Must(meter.Int64Histogram("remote_write_processing_latency_ns",
			metric.WithDescription("Latency of remote write processing in nanoseconds per timeseries"),
			metric.WithUnit("ns"),
		)),

		RemoteWriteTotalProcessedSeries: util.Must(meter.Int64Counter("remote_write_total_processed_series",
			metric.WithDescription("Total number of series processed by remote write")),
		),
	}
}

func AggregationSelector(ik sdkmetric.InstrumentKind) aggregation.Aggregation {
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
