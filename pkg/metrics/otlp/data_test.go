package otlp_test

import (
	otlpcommonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsotlpv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
)

var (
	dropMeAttr = &otlpcommonv1.KeyValue{
		Key: "drop",
		Value: &otlpcommonv1.AnyValue{
			Value: &otlpcommonv1.AnyValue_StringValue{
				StringValue: "me",
			},
		},
	}
	notDropMeAttr = &otlpcommonv1.KeyValue{
		Key: "drop",
		Value: &otlpcommonv1.AnyValue{
			Value: &otlpcommonv1.AnyValue_StringValue{
				StringValue: "me",
			},
		},
	}
	dropScope = []*metricsotlpv1.ResourceMetrics{
		{
			ScopeMetrics: []*metricsotlpv1.ScopeMetrics{
				{
					Scope: &otlpcommonv1.InstrumentationScope{
						Attributes: []*otlpcommonv1.KeyValue{
							dropMeAttr,
						},
					},
					Metrics: []*metricsotlpv1.Metric{
						{
							Name:        "foo",
							Description: "bar",
							Unit:        "TODO",
							Data: &metricsotlpv1.Metric_Gauge{
								Gauge: &metricsotlpv1.Gauge{
									DataPoints: []*metricsotlpv1.NumberDataPoint{
										{
											Attributes: []*otlpcommonv1.KeyValue{
												dropMeAttr,
											},
										},
										{
											Attributes: []*otlpcommonv1.KeyValue{
												notDropMeAttr,
											},
										},
									},
								},
							},
						},
						{
							Name:        "foo",
							Description: "bar",
							Unit:        "TODO",
							Data: &metricsotlpv1.Metric_Sum{
								Sum: &metricsotlpv1.Sum{
									DataPoints: []*metricsotlpv1.NumberDataPoint{
										{
											Value: &metricsotlpv1.NumberDataPoint_AsInt{
												AsInt: 1,
											},
											Attributes: []*otlpcommonv1.KeyValue{
												notDropMeAttr,
											},
										},
										{
											Value: &metricsotlpv1.NumberDataPoint_AsInt{
												AsInt: 1,
											},
											Attributes: []*otlpcommonv1.KeyValue{
												notDropMeAttr,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	dropDataPoints = []*metricsotlpv1.ResourceMetrics{
		{
			ScopeMetrics: []*metricsotlpv1.ScopeMetrics{
				{
					Scope: &otlpcommonv1.InstrumentationScope{
						Attributes: []*otlpcommonv1.KeyValue{
							{
								Key: "notdrop",
								Value: &otlpcommonv1.AnyValue{
									Value: &otlpcommonv1.AnyValue_StringValue{
										StringValue: "me",
									},
								},
							},
						},
					},
					Metrics: []*metricsotlpv1.Metric{
						{
							Name:        "foo",
							Description: "bar",
							Unit:        "TODO",
							Data: &metricsotlpv1.Metric_Gauge{
								Gauge: &metricsotlpv1.Gauge{
									DataPoints: []*metricsotlpv1.NumberDataPoint{
										{
											Value: &metricsotlpv1.NumberDataPoint_AsInt{
												AsInt: 1,
											},
											Attributes: []*otlpcommonv1.KeyValue{
												notDropMeAttr,
											},
										},
										{
											Value: &metricsotlpv1.NumberDataPoint_AsInt{
												AsInt: 2,
											},
											Attributes: []*otlpcommonv1.KeyValue{
												notDropMeAttr,
											},
										},
									},
								},
							},
						},
						{
							Name:        "foo",
							Description: "bar",
							Unit:        "TODO",
							Data: &metricsotlpv1.Metric_Sum{
								Sum: &metricsotlpv1.Sum{
									DataPoints: []*metricsotlpv1.NumberDataPoint{
										{
											Value: &metricsotlpv1.NumberDataPoint_AsInt{
												AsInt: 1,
											},
											Attributes: []*otlpcommonv1.KeyValue{
												dropMeAttr,
											},
										},
										{
											Value: &metricsotlpv1.NumberDataPoint_AsInt{
												AsInt: 1,
											},
											Attributes: []*otlpcommonv1.KeyValue{
												dropMeAttr,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
)
