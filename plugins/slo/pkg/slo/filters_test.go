package slo_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/slo/pkg/slo"
)

var _ = Describe("SLO Filter tests", Ordered, Label(test.Unit, test.Slow), func() {
	{
		When("We use SLO filters", func() {
			It("should get parse them from our embedded directory definitions", func() {
				for dirName, embedFs := range slo.EnabledFilters {
					filters := slo.GetGroupConfigsFromEmbed(dirName, embedFs)
					Expect(filters).NotTo(HaveLen(0))
					for _, filter := range filters {
						Expect(filter.Name).NotTo(Equal(""))
						Expect(filter.Filters).NotTo(BeEmpty())
					}
				}
			})

			It("Should be able to score the events based on the filters", func() {
				// array of prom metrics : label name -> label vals
				labels := &cortexadmin.MetricLabels{
					Items: []*cortexadmin.LabelSet{
						{
							Name: "go_gc_duration_seconds",
						},
						{
							Name: "jvm_something",
						},
						{
							Name: "jvm_something_else",
						},
						{
							Name: "uptime_seconds",
						},
						{
							Name: "kube-proxy",
						},
						{
							Name: "apiserver_something",
						},
						{
							Name: "request_duration_seconds",
						},
						{
							Name: "cpu_usage_seconds_total",
						},
					},
				}

				filteredGroups, err := slo.ApplyFiltersToCortexEvents(labels)
				Expect(err).To(Succeed())
				Expect(filteredGroups).NotTo(BeNil())
				Expect(filteredGroups.GroupNameToEvent["golang events"].Items).To(HaveLen(1))
				Expect(filteredGroups.GroupNameToEvent["jvm events"].Items).To(HaveLen(2))
				Expect(filteredGroups.GroupNameToEvent["kubernetes events"].Items).To(HaveLen(1))
				Expect(filteredGroups.GroupNameToEvent["network events"].Items).To(HaveLen(1))
				Expect(filteredGroups.GroupNameToEvent["compute events"].Items).To(HaveLen(1))
				Expect(filteredGroups.GroupNameToEvent["other events"].Items).To(HaveLen(2))

			})
		})
	}
})
