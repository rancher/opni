package routing

import (
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/alerting/drivers/config"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
)

var OpniMetricsSubRoutingTreeId config.Matchers = []*labels.Matcher{
	alertingv1.OpniMetricsSubRoutingTreeMatcher,
}

// this subtree connects to cortex
func NewOpniMetricsSubtree() *config.Route {
	metricsRoute := &config.Route{
		// expand all labels in case our default grouping overshadows some of the user's configs
		GroupBy: []model.LabelName{
			"...",
		},
		Matchers: OpniMetricsSubRoutingTreeId,
		Routes:   []*config.Route{},
		Continue: true, // want to expand the subtree
	}
	setDefaultRateLimitingFromProto(metricsRoute)
	return metricsRoute
}
