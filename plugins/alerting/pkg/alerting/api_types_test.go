package alerting_test

import (
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
)

var _ = Describe("Internal alerting plugin functionality test", Ordered, Label(test.Unit, test.Slow), func() {
	When("When we set data into PostableAlert messages", func() {
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

		It("should validate valid PostableAlerts", func() {
			alert := &alerting.PostableAlert{}
			alert.WithCondition("web.hook")
			Expect(alert.Must()).To(Succeed())
		})

		It("should error on invalid PostableAlerts", func() {
			alert := &alerting.PostableAlert{}
			Expect(alert.Must()).To(HaveOccurred())
			alert.WithRuntimeInfo("foo", "bar")
			Expect(alert.Must()).To(HaveOccurred())
		})
	})

	When("We set data into PostableSilence/Delete silence messages", func() {
		It("Should set the condition id correctly for PostableSilence", func() {
			silence := &alerting.PostableSilence{}
			silence.WithCondition("web.hook")
			Expect(silence.Matchers).NotTo(BeNil())
			Expect(silence.Matchers).To(HaveLen(1))
			Expect(silence.Matchers[0].Name).To(Equal("web.hook"))
		})

		It("Should set the durations correctly PostableSilence", func() {
			approxDiff := time.Second * 1
			dur := time.Minute * 15
			expectedNow := time.Now()
			expectedStop := expectedNow.Add(dur)
			silence := &alerting.PostableSilence{}
			silence.WithDuration(dur)
			Expect(silence.StartsAt).NotTo(BeNil())
			Expect(silence.EndsAt).NotTo(BeNil())
			Expect(silence.StartsAt.Unix()).To(
				BeNumerically(">=", expectedNow.Unix()))
			Expect(silence.StartsAt.Unix()).To(
				BeNumerically("<=", expectedNow.Add(approxDiff).Unix()))
			Expect(silence.EndsAt.Unix()).To(
				BeNumerically(">=", expectedStop.Unix()))
			Expect(silence.EndsAt.Unix()).To(
				BeNumerically("<=", expectedStop.Add(approxDiff).Unix()))
		})

		It("should Must valid PostableSilences", func() {
			dur := time.Minute * 15
			silence := &alerting.PostableSilence{}
			silence.WithDuration(dur)
			silence.WithCondition("web.hook")
			Expect(silence.Must()).To(Succeed())
		})

		It("should errror on invalid PostableSilences", func() {
			dur := time.Minute * 15
			silence := &alerting.PostableSilence{}
			Expect(silence.Must()).To(HaveOccurred())
			silence.WithDuration(dur)
			Expect(silence.Must()).To(HaveOccurred())

			silence2 := &alerting.PostableSilence{}
			silence2.WithCondition("web.hook")
			Expect(silence2.Must()).To(HaveOccurred())
		})
	})
})
