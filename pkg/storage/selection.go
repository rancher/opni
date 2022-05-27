package storage

import corev1 "github.com/rancher/opni/pkg/apis/core/v1"

type SelectorPredicate func(*corev1.Cluster) bool

type ClusterSelector struct {
	ClusterIDs    []string
	LabelSelector *corev1.LabelSelector
	MatchOptions  corev1.MatchOptions
}

func (p ClusterSelector) Predicate() SelectorPredicate {
	emptyLabelSelector := p.LabelSelector.IsEmpty()
	if emptyLabelSelector && len(p.ClusterIDs) == 0 {
		switch {
		case p.MatchOptions&corev1.MatchOptions_EmptySelectorMatchesNone != 0:
			return func(cluster *corev1.Cluster) bool { return false }
		default:
			return func(c *corev1.Cluster) bool { return true }
		}
	}
	idSet := map[string]struct{}{}
	for _, id := range p.ClusterIDs {
		idSet[id] = struct{}{}
	}
	return func(c *corev1.Cluster) bool {
		id := c.Id
		if _, ok := idSet[id]; ok {
			return true
		}
		if emptyLabelSelector {
			return false
		}
		return labelSelectorMatches(p.LabelSelector, c.GetMetadata().GetLabels())
	}
}

func labelSelectorMatches(selector *corev1.LabelSelector, labels map[string]string) bool {
	for key, value := range selector.MatchLabels {
		if labels[key] != value {
			return false
		}
	}
	for _, req := range selector.MatchExpressions {
		switch corev1.LabelSelectorOperator(req.Operator) {
		case corev1.LabelSelectorOpIn:
			ok := false
			for _, value := range req.Values {
				if labels[req.Key] == value {
					ok = true
					break
				}
			}
			if !ok {
				return false
			}
		case corev1.LabelSelectorOpNotIn:
			if v, ok := labels[req.Key]; !ok {
				return false
			} else {
				for _, value := range req.Values {
					if v == value {
						return false
					}
				}
				return true
			}
		case corev1.LabelSelectorOpExists:
			if _, ok := labels[req.Key]; !ok {
				return false
			}
		case corev1.LabelSelectorOpDoesNotExist:
			if _, ok := labels[req.Key]; ok {
				return false
			}
		}
	}
	return true
}
