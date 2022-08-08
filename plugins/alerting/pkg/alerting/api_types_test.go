package alerting_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
)

var _ = Describe("Internal alerting plugin functionality test", Ordered, Label(test.Unit, test.Slow), func() {

	When("When we set data into PostableAlert ", func() {
		var err error
		Expect(err).To(BeNil())

		It("Should set the condition id correctly", func() {
			alert := &alerting.PostableAlert{}
			alert.WithCondition("web.hook")
			Expect(alert.Labels).NotTo(BeNil())
			Expect(alert.Labels["alertname"]).To(Equal("web.hook"))
		})

		It("Should set the runtimeInfo correctly", func() {
			alert := &alerting.PostableAlert{}
			alert.WithRuntimeInfo("foo", "bar")
			Expect(alert.Annotations).NotTo(BeNil())
			Expect((*alert.Annotations)["foo"]).To(Equal("bar"))
		})

		Specify("The PostableAlert should be able to be marshaled to json", func() {
			alert := &alerting.PostableAlert{}
			alert.WithCondition("web.hook")
			alert.WithRuntimeInfo("foo", "bar")
			arr := []*alerting.PostableAlert{alert}
			bytes, err := json.Marshal(arr)
			Expect(err).To(Succeed())
			Expect(bytes).NotTo(HaveLen(0))
		})
	})
})
