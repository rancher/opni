package metrics_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/slo/backend/metrics"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
)

var _ = Describe("SLO Filter tests", Ordered, Label("unit"), func() {
	When("We use SLO filters", func() {
		var filters []metrics.Filter
		It("should get parse them from our embedded directory definitions", func() {
			filters = make([]metrics.Filter, 0)
			for dirName, embedFs := range metrics.EnabledFilters {
				flts := metrics.GetGroupConfigsFromEmbed(dirName, embedFs)
				filters = append(filters, flts...)
			}
			for _, filter := range filters {
				Expect(filter.Name).NotTo(Equal(""))
				Expect(filter.Filters).NotTo(BeEmpty())
			}
			Expect(filters).To(HaveLen(6))
		})

		It("Should be able to score the events based on the filters", func() {
			// array of prom metrics : label name -> label vals
			series := &cortexadmin.SeriesInfoList{
				Items: []*cortexadmin.SeriesInfo{
					{
						SeriesName: "go_gc_duration_seconds",
					},
					{
						SeriesName: "jvm_something",
					},
					{
						SeriesName: "jvm_something_else",
					},
					{
						SeriesName: "uptime_seconds",
					},
					{
						SeriesName: "kube-proxy",
					},
					{
						SeriesName: "apiserver_something",
					},
					{
						SeriesName: "request_duration_seconds",
					},
					{
						SeriesName: "cpu_usage_seconds_total",
					},
					{
						SeriesName: ".*kube-prometheus-operator.*",
					},
				},
			}

			filteredGroupsResp := metrics.ApplyFiltersToMetricEvents(series, filters)
			Expect(filteredGroupsResp).NotTo(BeNil())

			filteredGroups := filteredGroupsResp.GetGroupNameToMetrics()

			Expect(filteredGroups).To(HaveKey("golang metrics"))
			Expect(filteredGroups).To(HaveKey("jvm metrics"))
			Expect(filteredGroups).To(HaveKey("kubernetes metrics"))
			Expect(filteredGroups).To(HaveKey("network metrics"))
			Expect(filteredGroups).To(HaveKey("compute metrics"))
			Expect(filteredGroups).To(HaveKey("other metrics"))
		})
	})
})
