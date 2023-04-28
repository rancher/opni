package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
