/*
Contains implementation details for slo ratio query metrics
*/

package query

import (
	"bytes"
	"html/template"
	"regexp"
)

type RatioQuery struct {
	query        template.Template
	metricFilter regexp.Regexp // how to assign the metric to the query
}

func (r RatioQuery) FillQueryTemplate(info templateExecutor) string {
	//TODO : implement
	return ""
}

func (r RatioQuery) GetMetricFilter() string {
	//TODO : implement
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

func (r RatioQuery) Construct() (string, error) {
	// TODO : implement
	var query bytes.Buffer
	if err := r.query.Execute(&query, r); err != nil {
		return "", err
	}
	return query.String(), nil
}
