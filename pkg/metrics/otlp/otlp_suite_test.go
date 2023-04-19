package otlp_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	metricsotlpv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
)

func TestOtlp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Otlp Suite")
}

func compat(otlpMetrics []*metricsotlpv1.ResourceMetrics) []*metricdata.ResourceMetrics {
	res := make([]*metricdata.ResourceMetrics, len(otlpMetrics))
	for i := range otlpMetrics {
		res[i] = &metricdata.ResourceMetrics{
			Resource:     &resource.Resource{},
			ScopeMetrics: []metricdata.ScopeMetrics{},
		}
		// for _, sm := range rm.ScopeMetrics {
		// 	newMdataScope := metricdata.ScopeMetrics{
		// 		Scope: instrumentation.Scope{
		// 			Name:    sm.Scope.Name,
		// 			Version: sm.Scope.Version,
		// 		},
		// 		Metrics: make([]metricdata.Metrics, len(sm.Metrics)),
		// 	}
		// 	for _, m := range sm.Metrics {
		// 		newMdataScope.Metrics = append(newMdataScope.Metrics, metricdata.Metrics{
		// 			Data: metricdata.Gauge[int64]{},
		// 		})
		// 	}
		// }
	}
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
