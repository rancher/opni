package backend_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	slov1 "github.com/rancher/opni/pkg/apis/slo/v1"
	"github.com/rancher/opni/pkg/slo/backend"
)

var _ = Describe("Event matching", Label("unit"), func() {
	When("we receive event data from the user", func() {
		It("should match empty total events to all events", func() {
			goodEvents := []*slov1.Event{
				{
					Key:  "code",
					Vals: []string{"200"},
				},
			}
			totalEvents := []*slov1.Event{}
			g, t := backend.MatchEventsOnMetric(goodEvents, totalEvents)
			Expect(g).To(Equal([]*slov1.Event{
				{
					Key:  "code",
					Vals: []string{"200"},
				},
			}))
			Expect(t).To(Equal([]*slov1.Event{}))
		})

		It("should include good label filters in matching total event filters ", func() {
			goodEvents := []*slov1.Event{
				{
					Key:  "code",
					Vals: []string{"200"},
				},
			}
			totalEvents := []*slov1.Event{
				{
					Key:  "code",
					Vals: []string{"500", "503"},
				},
			}
			g, t := backend.MatchEventsOnMetric(goodEvents, totalEvents)
			Expect(g).To(Equal(
				[]*slov1.Event{
					{
						Key:  "code",
						Vals: []string{"200"},
					},
				},
			))
			Expect(t).To(Equal(
				[]*slov1.Event{
					{
						Key:  "code",
						Vals: []string{"200", "500", "503"},
					},
				},
			))
		})

		It("should apply missing total label filters to good filters", func() {
			goodEvents := []*slov1.Event{
				{
					Key:  "code",
					Vals: []string{"200"},
				},
			}
			totalEvents := []*slov1.Event{
				{
					Key:  "handler",
					Vals: []string{"/ready"},
				},
			}
			g, t := backend.MatchEventsOnMetric(goodEvents, totalEvents)

			Expect(g).To(Equal(
				[]*slov1.Event{
					{
						Key:  "code",
						Vals: []string{"200"},
					},
					{
						Key:  "handler",
						Vals: []string{"/ready"},
					},
				},
			))
			Expect(t).To(Equal(
				[]*slov1.Event{
					{
						Key:  "handler",
						Vals: []string{"/ready"},
					},
				},
			))
		})

		It("should sanitize invalid inputs by omitting them", func() {
			goodEvents := []*slov1.Event{
				{
					Key:  "",
					Vals: []string{"200"},
				},
			}
			totalEvents := []*slov1.Event{
				{
					Key:  "",
					Vals: []string{"500", "503"},
				},
				{
					Key:  "handler",
					Vals: nil,
				},
			}

			g, t := backend.MatchEventsOnMetric(goodEvents, totalEvents)
			Expect(g).To(Equal([]*slov1.Event{}))
			Expect(t).To(Equal([]*slov1.Event{}))
		})
	})
})
