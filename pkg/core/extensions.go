package core

import "strings"

type LabelSelectorOperator string

const (
	LabelSelectorOpIn           LabelSelectorOperator = "In"
	LabelSelectorOpNotIn        LabelSelectorOperator = "NotIn"
	LabelSelectorOpExists       LabelSelectorOperator = "Exists"
	LabelSelectorOpDoesNotExist LabelSelectorOperator = "DoesNotExist"
)

func (ls *LabelSelector) ExpressionString() string {
	if ls == nil {
		return ""
	}
	expressions := make([]string, 0, len(ls.MatchLabels)+len(ls.MatchExpressions))
	for k, v := range ls.MatchLabels {
		expressions = append(expressions, (&LabelSelectorRequirement{
			Key:      k,
			Operator: string(LabelSelectorOpIn),
			Values:   []string{v},
		}).ExpressionString())
	}
	for _, expr := range ls.MatchExpressions {
		expressions = append(expressions, expr.ExpressionString())
	}

	return strings.Join(expressions, "&&")
}

func (lsr *LabelSelectorRequirement) ExpressionString() string {
	if lsr == nil {
		return ""
	}
	return lsr.Key + " " + lsr.Operator + " [" + strings.Join(lsr.Values, ",") + "]"
}
