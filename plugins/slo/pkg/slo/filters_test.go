package slo_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/slo/pkg/slo"
)

var _ = Describe("SLO Filter tests", Ordered, Label("unit", "slow"), func() {
	{
		When("We use SLO filters", func() {
			It("should get parse them from our embedded directory definitions", func() {
				for dirName, embedFs := range slo.EnabledFilters {
					filters := slo.GetGroupConfigsFromEmbed(logger.NewPluginLogger().WithGroup("slo"), dirName, embedFs)
					Expect(filters).NotTo(HaveLen(0))
					for _, filter := range filters {
						Expect(filter.Name).NotTo(Equal(""))
						Expect(filter.Filters).NotTo(BeEmpty())
					}
				}
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
					},
				}

				filteredGroups, err := slo.ApplyFiltersToCortexEvents(series)
				Expect(err).To(Succeed())
				Expect(filteredGroups).NotTo(BeNil())
				Expect(filteredGroups.GroupNameToMetrics["golang metrics"].Items).To(HaveLen(1))
				Expect(filteredGroups.GroupNameToMetrics["jvm metrics"].Items).To(HaveLen(2))
				Expect(filteredGroups.GroupNameToMetrics["kubernetes metrics"].Items).To(HaveLen(1))
				Expect(filteredGroups.GroupNameToMetrics["network metrics"].Items).To(HaveLen(1))
				Expect(filteredGroups.GroupNameToMetrics["compute metrics"].Items).To(HaveLen(1))
				Expect(filteredGroups.GroupNameToMetrics["other metrics"].Items).To(HaveLen(2))
			})
		})
	}
})
