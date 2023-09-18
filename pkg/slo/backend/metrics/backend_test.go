package metrics_test

import (
	"context"
	"encoding/json"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/common/model"
	slov1 "github.com/rancher/opni/pkg/apis/slo/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/metrics/compat"
	"github.com/rancher/opni/pkg/slo/backend/metrics"
	cortexadmin_mock "github.com/rancher/opni/pkg/test/mock/cortexadmin"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"google.golang.org/grpc"
)

var _ = Describe("Metrics SLO Service backend", Label("unit"), Ordered, func() {
	var mb *metrics.MetricsBackend
	BeforeAll(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockClient := cortexadmin_mock.NewMockCortexAdminClient(ctrl)
		mockClient.EXPECT().
			Query(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, req *cortexadmin.QueryRequest, _ ...grpc.CallOption) (*cortexadmin.QueryResponse, error) {
				v := model.Vector{
					{
						Metric: model.Metric{
							"job": "test-service",
						},
						Value:     1,
						Timestamp: model.Time(100000),
					},
				}

				wrapper := struct {
					Type   model.ValueType `json:"resultType"`
					Result json.RawMessage `json:"result"`
				}{}
				wrapper.Type = model.ValVector
				wrapperBytes, err := json.Marshal(v)
				if err != nil {
					return nil, nil
				}
				wrapper.Result = wrapperBytes

				vectorData, err := json.Marshal(wrapper)
				if err != nil {
					return nil, err
				}

				promResp := compat.ApiResponse{
					Status:    "success",
					Data:      vectorData,
					ErrorType: "",
					Error:     "",
					Warnings:  []string{},
				}
				data, err := json.Marshal(promResp)
				if err != nil {
					return nil, err
				}
				return &cortexadmin.QueryResponse{
					Data: data,
				}, nil
			}).AnyTimes()

		mockClient.EXPECT().
			GetSeriesMetrics(gomock.Any(), gomock.Any()).
			Return(&cortexadmin.SeriesInfoList{
				Items: []*cortexadmin.SeriesInfo{
					{
						SeriesName: "test",
					},
				},
			}, nil).AnyTimes()

		mockClient.EXPECT().
			GetMetricLabelSets(gomock.Any(), gomock.Any()).
			Return(&cortexadmin.MetricLabels{
				Items: []*cortexadmin.LabelSet{
					{
						Name:  "test-label",
						Items: []string{"host_name"},
					},
				},
			}, nil).AnyTimes()

		m := metrics.NewProvider(logger.NewPluginLogger().Named("slo"))
		m.Initialize(mockClient)
		mb = metrics.NewBackend(m)
	})
	When("we we use the metrics service backend", func() {
		It("should list discoverable services", func() {
			svcList, err := mb.ListServices(
				context.TODO(),
				&slov1.ListServicesRequest{
					Datasource: "metrics",
					ClusterId:  "test",
				})
			Expect(err).NotTo(HaveOccurred())
			Expect(svcList.Items).To(ContainElement(&slov1.Service{
				ClusterId: "test",
				ServiceId: "test-service",
			}))
		})

		It("should list metrics based on services", func() {
			metricList, err := mb.ListMetrics(
				context.TODO(),
				&slov1.ListMetricsRequest{
					Datasource: "metrics",
					ClusterId:  "test",
					ServiceId:  "test",
				},
			)
			Expect(err).NotTo(HaveOccurred())
			expected := &slov1.MetricGroupList{
				GroupNameToMetrics: map[string]*slov1.MetricList{
					"other metrics": {
						Items: []*slov1.Metric{{Id: "test", Metadata: &slov1.MetricMetadata{}}},
					},
				},
			}
			Expect(metricList).To(testutil.ProtoEqual(expected))
		})

		It("should list events based on  metrics and services", func() {
			eventList, err := mb.ListEvents(context.TODO(),
				&slov1.ListEventsRequest{
					Datasource: "metrics",
					ClusterId:  "test",
					ServiceId:  "test",
					MetricId:   "test",
				})
			Expect(err).NotTo(HaveOccurred())
			Expect(eventList).To(Equal(&slov1.EventList{
				Items: []*slov1.Event{
					{
						Key:  "test-label",
						Vals: []string{"host_name"},
					},
				},
			}))
		})
	})
})
