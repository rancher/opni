package core_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rancher/opni/pkg/core"
)

var _ = Describe("Labels", func() {
	selector := &core.LabelSelector{
		MatchLabels: map[string]string{
			"foo": "bar",
		},
		MatchExpressions: []*core.LabelSelectorRequirement{
			{
				Key:      "a",
				Operator: string(core.LabelSelectorOpIn),
				Values:   []string{"1", "2"},
			},
			{
				Key:      "b",
				Operator: string(core.LabelSelectorOpExists),
			},
			{
				Key:      "c",
				Operator: string(core.LabelSelectorOpDoesNotExist),
			},
			{
				Key:      "d",
				Operator: string(core.LabelSelectorOpNotIn),
				Values:   []string{"3", "4"},
			},
		},
	}

	It("should convert label selectors", func() {
		k8sSelector := selector.ToLabelSelector()
		Expect(k8sSelector.MatchLabels).To(Equal(selector.MatchLabels))
		for i, req := range k8sSelector.MatchExpressions {
			Expect(req.Key).To(Equal(selector.MatchExpressions[i].Key))
			Expect(string(req.Operator)).To(Equal(selector.MatchExpressions[i].Operator))
			Expect(req.Values).To(Equal(selector.MatchExpressions[i].Values))
		}

		Expect((*core.LabelSelector)(nil).ToLabelSelector()).To(Equal((*metav1.LabelSelector)(nil)))
	})

	It("should generate expression strings", func() {
		Expect(selector.ExpressionString()).To(Equal("foo ∈ {bar} && a ∈ {1,2} && ∃ b && ∄ c && d ∉ {3,4}"))
		Expect((*core.LabelSelector)(nil).ExpressionString()).To(Equal(""))
		Expect((&core.LabelSelector{
			MatchExpressions: []*core.LabelSelectorRequirement{nil, nil},
		}).ExpressionString()).To(Equal(""))
		Expect((&core.LabelSelector{
			MatchExpressions: []*core.LabelSelectorRequirement{
				{
					Key:      "a",
					Operator: "invalid",
					Values:   []string{"1", "2"},
				},
			},
		}).ExpressionString()).To(Equal("a ? {1,2}"))
		Expect((*core.LabelSelectorRequirement)(nil).ExpressionString()).To(Equal(""))
	})
})
