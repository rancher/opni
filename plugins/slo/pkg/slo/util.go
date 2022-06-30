package slo

import (
	"html/template"
	"time"

	prommodel "github.com/prometheus/common/model"
)

const (
	tplKeyWindow = "window"
)

var burnRateRecordingExprTpl = template.Must(template.New("burnRateExpr").Option("missingkey=error").Parse(`{{ .SLIErrorMetric }}{{ .MetricFilter }}
/ on({{ .SLOIDName }}, {{ .SLOLabelName }}, {{ .SLOServiceName }}) group_left
{{ .ErrorBudgetRatioMetric }}{{ .MetricFilter }}
`))

// Multiburn multiwindow alert template.
var mwmbAlertTpl = template.Must(template.New("mwmbAlertTpl").Option("missingkey=error").Parse(`(
    max({{ .QuickShortMetric }}{{ .MetricFilter}} > ({{ .QuickShortBurnFactor }} * {{ .ErrorBudgetRatio }})) without ({{ .WindowLabel }})
    and
    max({{ .QuickLongMetric }}{{ .MetricFilter}} > ({{ .QuickLongBurnFactor }} * {{ .ErrorBudgetRatio }})) without ({{ .WindowLabel }})
)
or
(
    max({{ .SlowShortMetric }}{{ .MetricFilter }} > ({{ .SlowShortBurnFactor }} * {{ .ErrorBudgetRatio }})) without ({{ .WindowLabel }})
    and
    max({{ .SlowQuickMetric }}{{ .MetricFilter }} > ({{ .SlowQuickBurnFactor }} * {{ .ErrorBudgetRatio }})) without ({{ .WindowLabel }})
)
`))

// Pretty simple durations for prometheus.
func timeDurationToPromStr(t time.Duration) string {
	return prommodel.Duration(t).String()
}

func MergeLabels(ms ...map[string]string) map[string]string {
	res := map[string]string{}
	for _, m := range ms {
		for k, v := range m {
			res[k] = v
		}
	}
	return res
}

func labelsToPromFilter(labels map[string]string) string {
	metricFilters := prommodel.LabelSet{}
	for k, v := range labels {
		metricFilters[prommodel.LabelName(k)] = prommodel.LabelValue(v)
	}

	return metricFilters.String()
}
