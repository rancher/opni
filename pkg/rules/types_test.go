package rules_test

import (
	"unsafe"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/model/rulefmt"

	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/test"
)

var _ = Describe("Types", func() {
	It("should clone rule groups", func() {
		rg, errs := rulefmt.Parse(test.TestData("prometheus/sample-rules.yaml"))
		Expect(errs).To(BeEmpty())
		groups := rg.Groups
		clone := rules.CloneRuleGroupList(groups)
		Expect(unsafe.Pointer(&clone)).NotTo(Equal(unsafe.Pointer(&groups)))
		Expect(unsafe.Pointer(&clone[0].Rules)).NotTo(Equal(unsafe.Pointer(&groups[0].Rules)))
		Expect(unsafe.Pointer(&clone[0].Rules[0].Labels)).NotTo(Equal(unsafe.Pointer(&groups[0].Rules[0].Labels)))
		Expect(unsafe.Pointer(&clone[0].Rules[0].Annotations)).NotTo(Equal(unsafe.Pointer(&groups[0].Rules[0].Annotations)))
	})
})
