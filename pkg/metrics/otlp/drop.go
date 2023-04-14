package otlp

import (
	metricsotlpv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
)

type DropConfig struct {
}

func UnsafeDrop(metrics []*metricsotlpv1.ResourceMetrics, dropCfg DropConfig) []*metricsotlpv1.ResourceMetrics {
	//TODO : implement me
	return metrics
}
