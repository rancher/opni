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

func (h HistogramQuery) FillQueryTemplate(info templateExecutor) (string, error) {
	var query bytes.Buffer
	if err := h.query.Execute(&query, info); err != nil {
		return "", err
	}
	return strings.TrimSpace(query.String()), nil
}

func (h HistogramQuery) GetMetricFilter() string {
	return h.metricFilter.String()
}

func (h HistogramQuery) Validate() error {
	return validatePromQl(h.query)
}

func (h HistogramQuery) IsRatio() bool {
	return false
}

func (h HistogramQuery) IsHistogram() bool {
	return true
}

func (h HistogramQuery) Construct(serv *api.ServiceInfo) (string, error) {
	return h.FillQueryTemplate(templateExecutor{
		MetricIdGood:  serv.MetricIdGood,
		MetricIdTotal: serv.MetricIdTotal,
		JobId:         serv.JobId,
	})
}

func (h HistogramQuery) BestMatch(in []string) string {
	return h.matcher(in)
}
