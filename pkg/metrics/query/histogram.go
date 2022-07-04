package query

import (
	"html/template"
	"regexp"

	slodef "github.com/rancher/opni/plugins/slo/pkg/slo"
)

type HistogramQuery struct {
	query        template.Template
	metricFilter regexp.Regexp
}

func (h HistogramQuery) FillQueryTemplate(info templateExecutor) string {
	// TODO : implement
	return ""
}

func (h HistogramQuery) GetMetricFilter() string {
	// TODO : implement
	return ""
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

func (h HistogramQuery) Construct() (string, error) {
	return "", slodef.ErrNotImplemented
}
