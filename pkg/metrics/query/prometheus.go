package query

/*
Make sure pre-configured metrics can be exported as valid prometheus/ promql
to the SLO api.
*/

import (
	"regexp"

	api "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	slodef "github.com/rancher/opni/plugins/slo/pkg/slo"
)

type PrometheusQueryImpl struct {
	name        string
	GoodQuery   Query
	TotalQuery  Query
	LabelRegex  regexp.Regexp
	datasource  string
	description string
}

// The actual metricId and window are only known at SLO creation time
func (p *PrometheusQueryImpl) Construct(service *api.Service) (*Query, error) {
	// var goodQuery bytes.Buffer
	// var totalQuery bytes.Buffer
	// if err := goodQueryTempl.Execute(&goodQuery,
	// 	goodQueryPlaceholder{MetricId: service.GetMetricId(), JobId: service.GetJobId(), Filter: p.GoodFilter}); err != nil {
	// 	return nil, err
	// }
	// if err := totalQueryTempl.Execute(
	// 	&totalQuery,
	// 	&service); err != nil {
	// 	return nil, err
	// }

	// return &RatioQuery{
	// 	GoodQuery:  strings.TrimSpace(goodQuery.String()),
	// 	TotalQuery: strings.TrimSpace(totalQuery.String()),
	// }, nil
	// TODO : implement
	return nil, slodef.ErrNotImplemented
}

func (p *PrometheusQueryImpl) Datasource() string {
	return p.datasource
}

func (p *PrometheusQueryImpl) Description() string {
	return p.description
}

func (p *PrometheusQueryImpl) Name() string {
	return p.name
}

func (p *PrometheusQueryImpl) ResolveLabel() *regexp.Regexp {
	return &p.LabelRegex
}
