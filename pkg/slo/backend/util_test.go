package backend_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/slo/backend"
)

var _ = Describe("SLO util", Label("unit"), func() {
	When("we use id labels util", func() {
		Specify("it should convert to prometheus join on filters", func() {
			i := backend.IdentificationLabels(map[string]string{
				"foo": "bar",
			})
			Expect(i.JoinOnPrometheus()).To(Equal("foo"))

			i2 := backend.IdentificationLabels(map[string]string{
				"foo": "bar",
				"baz": "qux",
			})

			Expect(i2.JoinOnPrometheus()).To(Equal("baz, foo"))
		})

		Specify("it should convert to SLO label pairs", func() {
			i := backend.IdentificationLabels(map[string]string{
				"foo": "bar",
				"baz": "qux",
			})

			Expect(i.ToLabels()).To(Equal(backend.LabelPairs{
				{
					Key:  "baz",
					Vals: []string{"qux"},
				},
				{
					Key:  "foo",
					Vals: []string{"bar"},
				},
			}))
		})
	})

	When("we use label pairs util", func() {
		Specify("it should construct prometheus join on filters", func() {
			lb := backend.LabelPairs{
				{
					Key:  "foo",
					Vals: []string{"bar"},
				},
				{
					Key:  "baz",
					Vals: []string{"qux", "qux2"},
				},
			}
			filter := lb.ConstructPrometheus()
			Expect(filter).To(Equal("foo=~\"bar\",baz=~\"qux|qux2\""))
		})
	})
})
