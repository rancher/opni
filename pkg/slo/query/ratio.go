/*
Contains implementation details for slo ratio query metrics
*/

package query

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"
	"text/template"

	"github.com/prometheus/client_golang/prometheus"
	api "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
)

type RatioQuery struct {
	query          template.Template
	metricFilter   regexp.Regexp // how to assign the metric to the query
	matcher        matcher
	goodCollector  *prometheus.Collector
	totalCollector *prometheus.Collector
	goodEvents     func(w http.ResponseWriter, r *http.Request)
	totalEvents    func(w http.ResponseWriter, r *http.Request)
}

func (rq RatioQuery) GetGoodCollector() prometheus.Collector {
	return *rq.goodCollector
}

func (rq RatioQuery) GetTotalCollector() prometheus.Collector {
	return *rq.totalCollector
}
func (rq RatioQuery) GetGoodEvents() func(w http.ResponseWriter, r *http.Request) {
	return rq.goodEvents
}
func (rq RatioQuery) GetTotalEvents() func(w http.ResponseWriter, r *http.Request) {
	return rq.totalEvents
}

func (r RatioQuery) FillQueryTemplate(info templateExecutor) (string, error) {
	var query bytes.Buffer
	if err := r.query.Execute(&query, info); err != nil {
		return "", err
	}
	return strings.TrimSpace(query.String()), nil
}

func (r RatioQuery) GetMetricFilter() string {
	return r.metricFilter.String()
}

func (r RatioQuery) Validate() error {
	return validatePromQl(r.query)
}

func (r RatioQuery) IsRatio() bool {
	return true
}

func (r RatioQuery) IsHistogram() bool {
	return false
}

func (r RatioQuery) Construct(serv *api.Service) (string, error) {
	return r.FillQueryTemplate(templateExecutor{
		MetricIdGood:  serv.MetricIdGood,
		MetricIdTotal: serv.MetricIdTotal,
		JobId:         serv.JobId,
	})
}

func (r RatioQuery) BestMatch(in []string) string {
	return r.matcher(in)
}
