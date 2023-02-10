package query

import (
	"text/template"
	// prommodel "github.com/prometheus/common/model"
)

// fill in query templates with mock information
var mockTemplateExecutor = templateExecutor{
	MetricIdGood:  "metric_id_good",
	MetricIdTotal: "metric_id_total",
	JobId:         "job_id",
}

type mockSlothInfo struct {
	window string
}

// fill in sloth IR with mock information
var requiredSlothInfo = mockSlothInfo{
	window: "1h",
}

func validatePromQl(_ template.Template) error {
	// var intermediate bytes.Buffer
	// queryTpl.Execute(&intermediate, mockTemplateExecutor)
	// // slothTmpl := template.Must(template.New("").Parse(intermediate.String()))
	// // var q bytes.Buffer
	// // slothTmpl.Execute(&q, requiredSlothInfo)
	// q := strings.ReplaceAll(intermediate.String(), "[{{.window}}]", "")
	//_, err := promqlparser.ParseExpr(q)
	// if err != nil {
	// 	return err
	// }
	return nil
}
