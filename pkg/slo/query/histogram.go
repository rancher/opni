package query

import (
	"bytes"
	"html/template"
	"regexp"
	"strings"

	api "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
)

type HistogramQuery struct {
	query        template.Template
	metricFilter regexp.Regexp
	matcher      matcher
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
	// TODO : implement
	return nil
}

func (h HistogramQuery) IsRatio() bool {
	return false
}

func (h HistogramQuery) IsHistogram() bool {
	return true
}

func (h HistogramQuery) Construct(serv *api.Service) (string, error) {
	return h.FillQueryTemplate(templateExecutor{
		MetricIdGood:  serv.MetricIdGood,
		MetricIdTotal: serv.MetricIdTotal,
		JobId:         serv.JobId,
	})
}

func (h HistogramQuery) BestMatch(in []string) string {
	return h.matcher(in)
}
