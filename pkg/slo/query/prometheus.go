package query

/*
Make sure pre-configured metrics can be exported as valid prometheus/ promql
to the SLO api.
*/

import (
	"net/http"
	"regexp"

	"github.com/prometheus/client_golang/prometheus"
	api "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
)

type PrometheusQueryImpl struct {
	name          string
	GoodQuery     Query
	TotalQuery    Query
	LabelRegex    regexp.Regexp
	datasource    string
	description   string
	collector     prometheus.Collector
	goodEventsGen func(w http.ResponseWriter, r *http.Request)
	badEventsGen  func(w http.ResponseWriter, r *http.Request)
}

// The actual metricId and window are only known at SLO creation time
func (p *PrometheusQueryImpl) Construct(service *api.Service) (*SLOQueryResult, error) {
	goodQueryStr, err := p.GoodQuery.Construct(service)
	if err != nil {
		return nil, err
	}
	totalQueryStr, err := p.TotalQuery.Construct(service)
	if err != nil {
		return nil, err
	}
	return &SLOQueryResult{GoodQuery: goodQueryStr, TotalQuery: totalQueryStr}, nil
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

func (p *PrometheusQueryImpl) GetGoodQuery() Query {
	return p.GoodQuery
}

func (p *PrometheusQueryImpl) GetTotalQuery() Query {
	return p.TotalQuery
}

func (p *PrometheusQueryImpl) GetCollector() prometheus.Collector {
	return p.collector
}

func (p *PrometheusQueryImpl) GetGoodEventGenerator() func(w http.ResponseWriter, r *http.Request) {
	return p.goodEventsGen
}
func (p *PrometheusQueryImpl) GetBadEventGenerator() func(w http.ResponseWriter, r *http.Request) {
	return p.badEventsGen
}
