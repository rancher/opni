package configutil_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/rancher/opni/pkg/alerting/configutil"
	alertingv2 "github.com/rancher/opni/pkg/apis/alerting/v2"
)

var _ = Describe("Label Matchers", func() {
	When("We use the protobuf's", func() {
		It("should convert alertingv2.LabelMatchers to labels.Matchers", func() {
			eq := &alertingv2.LabelMatcher{
				Type:  alertingv2.LabelMatcher_EQ,
				Name:  "foo",
				Value: "bar",
			}

			eqma, err := configutil.ToLabelMatchers([]*alertingv2.LabelMatcher{eq})
			Expect(err).To(Succeed())
			Expect(eqma).To(HaveLen(1))
			Expect(eqma[0].Name).To(Equal("foo"))
			Expect(eqma[0].Value).To(Equal("bar"))
			Expect(int(eqma[0].Type)).To(Equal(int(labels.MatchEqual)))

			ne := &alertingv2.LabelMatcher{
				Type:  alertingv2.LabelMatcher_NEQ,
				Name:  "foo",
				Value: "bar",
			}

			nema, err := configutil.ToLabelMatchers([]*alertingv2.LabelMatcher{ne})
			Expect(err).To(Succeed())
			Expect(nema).To(HaveLen(1))
			Expect(nema[0].Name).To(Equal("foo"))
			Expect(nema[0].Value).To(Equal("bar"))
			Expect(int(nema[0].Type)).To(Equal(int(labels.MatchNotEqual)))

			re := &alertingv2.LabelMatcher{
				Type:  alertingv2.LabelMatcher_RE,
				Name:  "foo",
				Value: "bar",
			}

			rema, err := configutil.ToLabelMatchers([]*alertingv2.LabelMatcher{re})
			Expect(err).To(Succeed())
			Expect(rema).To(HaveLen(1))
			Expect(rema[0].Name).To(Equal("foo"))
			Expect(rema[0].Value).To(Equal("bar"))
			Expect(int(rema[0].Type)).To(Equal(int(labels.MatchRegexp)))

			nre := &alertingv2.LabelMatcher{
				Type:  alertingv2.LabelMatcher_NRE,
				Name:  "foo",
				Value: "bar",
			}

			nrema, err := configutil.ToLabelMatchers([]*alertingv2.LabelMatcher{nre})
			Expect(err).To(Succeed())

			Expect(nrema).To(HaveLen(1))
			Expect(nrema[0].Name).To(Equal("foo"))
			Expect(nrema[0].Value).To(Equal("bar"))
			Expect(int(nrema[0].Type)).To(Equal(int(labels.MatchNotRegexp)))
		})

		It("should convert alertingv2.Labels to model.LabelSet", func() {
			labelSet := configutil.ToLabelSet(&alertingv2.Labels{})
			Expect(labelSet).To(BeEmpty())

			labelSet2 := configutil.ToLabelSet(&alertingv2.Labels{
				Labels: []*alertingv2.Label{},
			})
			Expect(labelSet2).To(BeEmpty())

			labelSet3 := configutil.ToLabelSet(&alertingv2.Labels{
				Labels: []*alertingv2.Label{
					{
						Name:  "foo",
						Value: "bar",
					},
					{
						Name:  "foo2",
						Value: "bar2",
					},
				},
			})
			Expect(labelSet3).To(HaveLen(2))
			Expect(string(labelSet3["foo"])).To(Equal("bar"))
			Expect(string(labelSet3["foo2"])).To(Equal("bar2"))
		})
	})
})
