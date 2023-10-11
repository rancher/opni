package metrics

import (
	"sync"

	"go.opentelemetry.io/otel/metric"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

var (
	meterProviderMu         sync.Mutex
	meterProvider           *sdkmetric.MeterProvider
	ActivationStatusCounter metric.Int64Counter
	SyncCycleProcessLatency metric.Int64Histogram
	SyncCycleStatusCounter  metric.Int64Counter
)

func RegisterMeterProvider(mp *sdkmetric.MeterProvider) {
	meterProviderMu.Lock()
	defer meterProviderMu.Unlock()
	meterProvider = mp
	createMetrics()
}

func createMetrics() {
	meter := meterProvider.Meter("plugin_alerting")

	activationStatusCounter, err := meter.Int64Counter(
		"alerting_alarm_activation_count",
		metric.WithDescription("Total number of alerting alarm activations"),
	)
	if err != nil {
		panic(err)
	}
	ActivationStatusCounter = activationStatusCounter
	syncCycleProcessLatency, err := meter.Int64Histogram(
		"alerting_sync_cycle_processing_latency",
		metric.WithDescription("Latency of alerting sync cycles in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		panic(err)
	}
	SyncCycleProcessLatency = syncCycleProcessLatency

	syncCycleStatusCounter, err := meter.Int64Counter(
		"alerting_sync_cycle_status_count",
		metric.WithDescription("Total number of alerting sync cycles"),
	)
	if err != nil {
		panic(err)
	}
	SyncCycleStatusCounter = syncCycleStatusCounter
}

func init() {
	meterProviderMu.Lock()
	defer meterProviderMu.Unlock()
	if meterProvider == nil {
		meterProvider = sdkmetric.NewMeterProvider()
	}
	createMetrics()
}
