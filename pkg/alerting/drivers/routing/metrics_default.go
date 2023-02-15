package routing

import (
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/alerting/shared"
)

var OpniMetricsSubRoutingTreeId config.Matchers = []*labels.Matcher{
	shared.OpniMetricsSubRoutingTreeMatcher,
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
