package slo

import (
	"text/template"
	"time"

	prommodel "github.com/prometheus/common/model"
)

const (
	tplKeyWindow = "window"
)

// Reference : https://github.com/slok/sloth/blob/eddd8145a696c3dc6d423e9d50cdb906186a52a3/internal/prometheus/recording_rules.go#L308
var burnRateRecordingExprTpl = template.Must(template.New("burnRateExpr").Option("missingkey=error").Parse(`{{ .SLIErrorMetric }}{{ .MetricFilter }}
/ on({{ .SLOIDName }}, {{ .SLOLabelName }}, {{ .SLOServiceName }}) group_left
{{ .ErrorBudgetRatioMetric }}{{ .MetricFilter }}
`))

// Reference : https://github.com/slok/sloth/blob/2de193572284e36189fe78ab33beb7e2b339b0f8/internal/prometheus/alert_rules.go#L109
// Multiburn multiwindow alert template.
var mwmbAlertTpl = template.Must(template.New("mwmbAlertTpl").Option("missingkey=error").Parse(`(max({{ .QuickShortMetric }}{{ .MetricFilter}} > bool ({{ .QuickShortBurnFactor }} * {{ .ErrorBudgetRatio }})) without ({{ .WindowLabel }}) and max({{ .QuickLongMetric }}{{ .MetricFilter}} > bool ({{ .QuickLongBurnFactor }} * {{ .ErrorBudgetRatio }})) without ({{ .WindowLabel }})) or (max({{ .SlowShortMetric }}{{ .MetricFilter }} > bool ({{ .SlowShortBurnFactor }} * {{ .ErrorBudgetRatio }})) without ({{ .WindowLabel }}) and max({{ .SlowQuickMetric }}{{ .MetricFilter }} > bool ({{ .SlowQuickBurnFactor }} * {{ .ErrorBudgetRatio }})) without ({{ .WindowLabel }}))`))

// Pretty simple durations for prometheus.
func TimeDurationToPromStr(t time.Duration) string {
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
