package metrics_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rancher/opni/plugins/metrics/pkg/agent"
	"github.com/samber/lo"
)

var _ = Describe("Chunks", Label("unit"), func() {
	When("request is empty", func() {
		It("should return same response", func() {
			requests, err := agent.FitRequestToSize(&prompb.WriteRequest{}, 100)
			Expect(err).ToNot(HaveOccurred())
			Expect(requests).To(Equal([]*prompb.WriteRequest{{}}))
		})
	})

	When("request is smalller than max", func() {
		It("should return the same response", func() {
			requests, err := agent.FitRequestToSize(&prompb.WriteRequest{
				Timeseries: []prompb.TimeSeries{
					{
						Labels: []prompb.Label{
							{
								Name:  "testLabel",
								Value: "testValue",
							},
						},
						Samples: []prompb.Sample{
							{
								Value:     .5,
								Timestamp: 100,
							},
						},
					},
				},
			}, 100)
			Expect(err).ToNot(HaveOccurred())
			Expect(requests).To(Equal([]*prompb.WriteRequest{
				{
					Timeseries: []prompb.TimeSeries{
						{
							Labels: []prompb.Label{
								{
									Name:  "testLabel",
									Value: "testValue",
								},
							},
							Samples: []prompb.Sample{
								{
									Value:     .5,
									Timestamp: 100,
								},
							},
						},
					},
				},
			}))

		})
	})

	When("request is larger than max", func() {
		It("should break the request into chunks", func() {
			requests, err := agent.FitRequestToSize(&prompb.WriteRequest{
				Timeseries: []prompb.TimeSeries{
					{
						Labels: []prompb.Label{
							{
								Name:  "testLabel",
								Value: "testValue",
							},
						},
						Samples: lo.Map(make([]prompb.Sample, 10), func(sample prompb.Sample, _ int) prompb.Sample {
							return prompb.Sample{
								Value:     .5,
								Timestamp: 100,
							}
						}),
					},
				},
			}, 100)

			expected := lo.Map(make([]any, 2), func(_ any, _ int) *prompb.WriteRequest {
				return &prompb.WriteRequest{
					Timeseries: []prompb.TimeSeries{
						{
							Labels: []prompb.Label{
								{
									Name:  "testLabel",
									Value: "testValue",
								},
							},
							Samples: lo.Map(make([]any, 5), func(_ any, _ int) prompb.Sample {
								return prompb.Sample{
									Value:     .5,
									Timestamp: 100,
								}
							}),
						},
					},
				}
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(requests).To(Equal(expected))
		})
	})

	When("max size is too small", func() {
		It("should fail", func() {
			requests, err := agent.FitRequestToSize(&prompb.WriteRequest{
				Timeseries: []prompb.TimeSeries{
					{
						Labels: []prompb.Label{
							{
								Name:  "testLabel",
								Value: "testValue",
							},
						},
						Samples: []prompb.Sample{
							{
								Value:     .5,
								Timestamp: 100,
							},
						},
					},
				},
			}, 1)
			Expect(err).To(HaveOccurred())
			Expect(requests).To(BeNil())
		})
	})
})
