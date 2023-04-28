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

func (rq RatioQuery) FillQueryTemplate(info templateExecutor) (string, error) {
	var query bytes.Buffer
	if err := rq.query.Execute(&query, info); err != nil {
		return "", err
	}
	return strings.TrimSpace(query.String()), nil
}

func (rq RatioQuery) GetMetricFilter() string {
	return rq.metricFilter.String()
}

func (rq RatioQuery) Validate() error {
	return validatePromQl(rq.query)
}

func (rq RatioQuery) IsRatio() bool {
	return true
}

func (rq RatioQuery) IsHistogram() bool {
	return false
}

func (rq RatioQuery) Construct(serv ServiceInfo) (string, error) {
	return rq.FillQueryTemplate(templateExecutor{
		MetricIdGood:  serv.GetMetricIdGood(),
		MetricIdTotal: serv.GetMetricIdTotal(),
		JobId:         serv.GetJobId(),
	})
}

func (rq RatioQuery) BestMatch(in []string) string {
	return rq.matcher(in)
}
