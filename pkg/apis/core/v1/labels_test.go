package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/rancher/opni/pkg/apis/core/v1"
)

var _ = Describe("Labels", Label("unit"), func() {
	selector := &v1.LabelSelector{
		MatchLabels: map[string]string{
			"foo": "bar",
		},
		MatchExpressions: []*v1.LabelSelectorRequirement{
			{
				Key:      "a",
				Operator: string(v1.LabelSelectorOpIn),
				Values:   []string{"1", "2"},
			},
			{
				Key:      "b",
				Operator: string(v1.LabelSelectorOpExists),
			},
			{
				Key:      "c",
				Operator: string(v1.LabelSelectorOpDoesNotExist),
			},
			{
				Key:      "d",
				Operator: string(v1.LabelSelectorOpNotIn),
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

		Expect((*v1.LabelSelector)(nil).ToLabelSelector()).To(Equal((*metav1.LabelSelector)(nil)))
	})

	It("should generate expression strings", func() {
		Expect(selector.ExpressionString()).To(Equal("foo ∈ {bar} && a ∈ {1,2} && ∃ b && ∄ c && d ∉ {3,4}"))
		Expect((*v1.LabelSelector)(nil).ExpressionString()).To(Equal(""))
		Expect((&v1.LabelSelector{
			MatchExpressions: []*v1.LabelSelectorRequirement{nil, nil},
		}).ExpressionString()).To(Equal(""))
		Expect((&v1.LabelSelector{
			MatchExpressions: []*v1.LabelSelectorRequirement{
				{
					Key:      "a",
					Operator: "invalid",
					Values:   []string{"1", "2"},
				},
			},
		}).ExpressionString()).To(Equal("a ? {1,2}"))
		Expect((*v1.LabelSelectorRequirement)(nil).ExpressionString()).To(Equal(""))
	})
})
