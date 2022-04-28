package core

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type LabelSelectorOperator string

const (
	LabelSelectorOpIn           LabelSelectorOperator = "In"
	LabelSelectorOpNotIn        LabelSelectorOperator = "NotIn"
	LabelSelectorOpExists       LabelSelectorOperator = "Exists"
	LabelSelectorOpDoesNotExist LabelSelectorOperator = "DoesNotExist"

	NameLabel = "opni.io/name"
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
		if expr == nil {
			continue
		}
		expressions = append(expressions, expr.ExpressionString())
	}

	return strings.Join(expressions, " && ")
}

func (lsr *LabelSelectorRequirement) ExpressionString() string {
	if lsr == nil {
		return ""
	}
	switch lsr.Operator {
	case string(LabelSelectorOpExists), string(LabelSelectorOpDoesNotExist):
		return keyWithOperatorSymbol(lsr.Key, lsr.Operator)
	default:
		return keyWithOperatorSymbol(lsr.Key, lsr.Operator) + " {" + strings.Join(lsr.Values, ",") + "}"
	}
}

func keyWithOperatorSymbol(key string, operator string) string {
	switch LabelSelectorOperator(operator) {
	case LabelSelectorOpIn:
		return key + " ∈"
	case LabelSelectorOpNotIn:
		return key + " ∉"
	case LabelSelectorOpExists:
		return "∃ " + key
	case LabelSelectorOpDoesNotExist:
		return "∄ " + key
	default:
		return key + " ?"
	}
}

func (ls *LabelSelector) ToLabelSelector() *metav1.LabelSelector {
	if ls == nil {
		return nil
	}
	s := &metav1.LabelSelector{
		MatchLabels:      ls.MatchLabels,
		MatchExpressions: []metav1.LabelSelectorRequirement{},
	}
	for _, expr := range ls.MatchExpressions {
		s.MatchExpressions = append(s.MatchExpressions, metav1.LabelSelectorRequirement{
			Key:      expr.Key,
			Operator: metav1.LabelSelectorOperator(expr.Operator),
			Values:   expr.Values,
		})
	}
	return s
}

func (ls *LabelSelector) IsEmpty() bool {
	return ls == nil || (len(ls.MatchLabels) == 0 && len(ls.MatchExpressions) == 0)
}
