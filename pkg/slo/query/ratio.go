/*
Contains implementation details for slo ratio query metrics
*/

package query

import (
	"bytes"
	"html/template"
	"regexp"
	"strings"

	api "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
)

type RatioQuery struct {
	query        template.Template
	metricFilter regexp.Regexp // how to assign the metric to the query
	matcher      matcher
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
	//TODO : implement
	return nil
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
