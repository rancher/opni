package desc_test

import (
	"unsafe"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	client_model "github.com/prometheus/client_model/go"

	"github.com/rancher/opni/pkg/metrics/desc"
	"github.com/rancher/opni/pkg/util"
)

var _ = Describe("Desc", Label("unit"), func() {
	It("should convert between Desc types", func() {
		d := &desc.Desc{
			FQName: "foo",
			Help:   "bar",
			ConstLabelPairs: []*client_model.LabelPair{
				{
					Name:  util.Pointer("foo"),
					Value: util.Pointer("bar"),
				},
			},
			VariableLabels: []string{"foo", "bar"},
			ID:             123,
			DimHash:        456,
		}
		d2 := desc.FromPrometheusDesc(d.ToPrometheusDesc())

		Expect(d).To(Equal(d2)) // Proto internal fields will not be set
		Expect(unsafe.Pointer(d)).NotTo(Equal(unsafe.Pointer(d2)))
		Expect(unsafe.Pointer(d.ConstLabelPairs[0])).NotTo(Equal(unsafe.Pointer(d2.ConstLabelPairs[0])))

		promD := d.ToPrometheusDescUnsafe()
		Expect(uintptr(unsafe.Pointer(promD))).To(Equal(
			uintptr(unsafe.Pointer(d)) + uintptr(unsafe.Offsetof(d.FQName))))
	})
})
