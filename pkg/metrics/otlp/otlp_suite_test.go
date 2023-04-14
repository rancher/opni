package otlp_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	metricsotlpv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
)

func TestOtlp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Otlp Suite")
}

func compat(otlpMetrics []*metricsotlpv1.ResourceMetrics) []*metricdata.ResourceMetrics {
	res := make([]*metricdata.ResourceMetrics, len(otlpMetrics))
	return res
}

func Validate(
	ctx context.Context,
	m []*metricdata.ResourceMetrics,
	opts ...metric.ManualReaderOption,
) error {
	reader := metric.NewManualReader(opts...)
	for _, rm := range m {
		if err := reader.Collect(ctx, rm); err != nil {
			return err
		}
	}
	return nil
}
