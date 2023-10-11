package configutil

import (
	"fmt"

	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/common/model"
	alertingv2 "github.com/rancher/opni/pkg/apis/alerting/v2"
	"github.com/samber/lo"
)

func convertEnum(in alertingv2.LabelMatcher_Type) (labels.MatchType, error) {
	switch in {
	case alertingv2.LabelMatcher_EQ:
		return labels.MatchEqual, nil
	case alertingv2.LabelMatcher_NEQ:
		return labels.MatchNotEqual, nil
	case alertingv2.LabelMatcher_RE:
		return labels.MatchRegexp, nil
	case alertingv2.LabelMatcher_NRE:
		return labels.MatchNotRegexp, nil
	}
	return labels.MatchEqual, fmt.Errorf("unsupported label type %s", in)
}

func ToLabelMatchers(lms []*alertingv2.LabelMatcher) (labels.Matchers, error) {
	res := make(labels.Matchers, len(lms))
	for i := 0; i < len(lms); i++ {

		ma, err := convertEnum(lms[i].Type)
		if err != nil {
			return nil, err
		}
		res[i], err = labels.NewMatcher(ma, lms[i].Name, lms[i].Value)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

func ToLabelSet(ls *alertingv2.Labels) model.LabelSet {
	lArr := ls.Labels
	labelSet := lo.Associate(lArr, func(l *alertingv2.Label) (model.LabelName, model.LabelValue) {
		return model.LabelName(l.Name), model.LabelValue(l.Value)
	})
	return labelSet
}
