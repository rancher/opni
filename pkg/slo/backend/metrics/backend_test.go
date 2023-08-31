package metrics_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	slov1 "github.com/rancher/opni/pkg/apis/slo/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/slo/backend/metrics"
	cortexadmin_mock "github.com/rancher/opni/pkg/test/mock/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
)

var _ = Describe("Metrics SLO Service backend", Label("unit"), Ordered, func() {
	var mb *metrics.MetricsBackend
	BeforeAll(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockClient := cortexadmin_mock.NewMockCortexAdminClient(ctrl)

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
		XIt("should list discoverable services", func() { //FIXME: TODO:
			svcList, err := mb.ListServices(
				context.TODO(),
				&slov1.ListServicesRequest{
					Datasource: "metrics",
					ClusterId:  "test",
				})
			Expect(err).NotTo(HaveOccurred())
			Expect(svcList.Items).To(ContainElement(&slov1.Service{
				ClusterId: "test",
				ServiceId: "test",
			}))
		})

		XIt("should list metrics based on services", func() {
			metricList, err := mb.ListMetrics(
				context.TODO(),
				&slov1.ListMetricsRequest{
					Datasource: "metrics",
					ClusterId:  "test",
					ServiceId:  "test",
				},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(metricList).To(Equal(&slov1.MetricGroupList{
				GroupNameToMetrics: map[string]*slov1.MetricList{},
			}))
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
