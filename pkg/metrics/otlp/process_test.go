package otlp_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/metrics/otlp"
	metricsotlpv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
)

func validateUnsafeDrop(metrics []*metricsotlpv1.ResourceMetrics, dropCfg otlp.DropConfig, expectedError error) {
	if expectedError == nil {
		Expect(
			Validate(
				context.TODO(),
				compat(otlp.UnsafeDrop(metrics, dropCfg)),
			),
		).To(Succeed())
	} else {
		Expect(
			Validate(
				context.TODO(),
				compat(otlp.UnsafeDrop(metrics, dropCfg)),
			),
		).To(MatchError(expectedError))
	}
}

func validateAggregate(
	metrics []*metricsotlpv1.ResourceMetrics,
	cfg otlp.AggregationConfig,
	expectedError error,
) {
	if expectedError == nil {
		Expect(
			Validate(
				context.TODO(),
				compat(otlp.Aggregate(metrics, cfg)),
			),
		).To(Succeed())
	} else {
		Expect(
			Validate(
				context.TODO(),
				compat(otlp.Aggregate(metrics, cfg)),
			),
		).To(MatchError(expectedError))
	}
}

var _ = Describe("OTLP metrics processing", Label("unit"), func() {
	DescribeTable("UnsafeDrop", validateUnsafeDrop,
		Entry(
			"no metrics/ no drop regex",
			[]*metricsotlpv1.ResourceMetrics{},
			otlp.DropConfig{},
			nil,
		),
		Entry(
			"dropping entire metric scopes",
			dropScope,
			otlp.DropConfig{
				DropCfg: []otlp.DropCondition{
					{
						Op:    otlp.OpEquals,
						Value: "drop",
					},
				},
			},
			nil,
		),
		Entry(
			"dropping entire metric datapoints",
			dropDataPoints,
			otlp.DropConfig{
				DropCfg: []otlp.DropCondition{
					{
						Op:    otlp.OpEquals,
						Value: "drop",
					},
				},
			},
			nil,
		),
	)
	DescribeTable("Aggregate", validateAggregate,
		Entry(
			"no metrics / no relabel to aggregate",
			[]*metricsotlpv1.ResourceMetrics{},
			otlp.AggregationConfig{},
			nil,
		),
	)
})
