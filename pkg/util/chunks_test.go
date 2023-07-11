package util_test

import (
	"runtime"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
)

var _ = Describe("Chunks", Serial, Label("unit"), func() {
	When("request is empty", func() {
		It("should return same response", func() {
			requests, err := util.SplitChunksWithLimit(&prompb.WriteRequest{}, util.DefaultWriteLimit())
			Expect(err).ToNot(HaveOccurred())
			Expect(requests).To(Equal([]*prompb.WriteRequest{{}}))
		})
	})

	When("request is under limt", func() {
		It("should return the same response", func() {
			requests, err := util.SplitChunksWithLimit(&prompb.WriteRequest{
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
			}, util.DefaultWriteLimit())
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

	When("request exceeds limits", func() {
		It("should break the request into chunks", func() {
			requests, err := util.SplitChunksWithLimit(&prompb.WriteRequest{
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
			}, util.WriteLimit{
				GrpcMaxBytes:             4194304,
				CortexIngestionRateLimit: 5,
			})

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

	When("request is massive", func() {
		It("should stay under memory limit", func() {
			var start, current, end runtime.MemStats

			experiment := gmeasure.NewExperiment("memory test")
			AddReportEntry(experiment.Name, experiment)

			runtime.GC()
			runtime.ReadMemStats(&start)

			experiment.RecordValue("start", float64(start.Alloc/1024))

			done := make(chan struct{})

			go func() {
				for {
					select {
					case <-done:
						return
					default:
						runtime.ReadMemStats(&current)
						experiment.RecordValue("runtime", float64(current.Alloc/1024))
						time.Sleep(time.Millisecond * 100)
					}
				}
			}()

			requests, err := util.SplitChunksWithLimit(&prompb.WriteRequest{
				Timeseries: []prompb.TimeSeries{
					{
						Labels: []prompb.Label{
							{
								Name:  "__name__",
								Value: "test_metric",
							},
						},
						Samples: lo.Map(make([]prompb.Sample, 4194304), func(sample prompb.Sample, i int) prompb.Sample {
							sample.Timestamp = time.Now().UnixMilli()
							return sample
						}),
					},
				},
			}, util.DefaultWriteLimit())
			Expect(err).ToNot(HaveOccurred())
			Expect(len(requests)).To(BeNumerically(">", 1))

			close(done)

			runtime.ReadMemStats(&end)
			experiment.RecordValue("end", float64(end.Alloc/1024))
		})
	})
})
