package query

import (
	"bytes"
	"net/http"
	"regexp"
	"strings"
	"text/template"

	"github.com/prometheus/client_golang/prometheus"
)

type HistogramQuery struct {
	query          template.Template
	metricFilter   regexp.Regexp
	matcher        matcher
	goodCollector  *prometheus.Collector
	totalCollector *prometheus.Collector
	goodEvents     func(w http.ResponseWriter, r *http.Request)
	totalEvents    func(w http.ResponseWriter, r *http.Request)
}

func (hq HistogramQuery) GetGoodCollector() prometheus.Collector {
	return *hq.goodCollector
}

func (hq HistogramQuery) GetTotalCollector() prometheus.Collector {
	return *hq.totalCollector
}
func (hq HistogramQuery) GetGoodEvents() func(w http.ResponseWriter, r *http.Request) {
	return hq.goodEvents
}
func (hq HistogramQuery) GetTotalEvents() func(w http.ResponseWriter, r *http.Request) {
	return hq.totalEvents
}

func (hq HistogramQuery) FillQueryTemplate(info templateExecutor) (string, error) {
	var query bytes.Buffer
	if err := hq.query.Execute(&query, info); err != nil {
		return "", err
	}
	return strings.TrimSpace(query.String()), nil
}

func (hq HistogramQuery) GetMetricFilter() string {
	return hq.metricFilter.String()
}

func (hq HistogramQuery) Validate() error {
	return validatePromQl(hq.query)
}

func (hq HistogramQuery) IsRatio() bool {
	return false
}

func (hq HistogramQuery) IsHistogram() bool {
	return true
}

func (hq HistogramQuery) Construct(serv ServiceInfo) (string, error) {
	return hq.FillQueryTemplate(templateExecutor{
		MetricIdGood:  serv.GetMetricIdGood(),
		MetricIdTotal: serv.GetMetricIdTotal(),
		JobId:         serv.GetJobId(),
	})
}

func (hq HistogramQuery) BestMatch(in []string) string {
	return hq.matcher(in)
}
