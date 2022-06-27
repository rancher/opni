package slo

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"regexp"
	"strings"

	"github.com/hashicorp/go-hclog"
	api "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
)

var (
	availableQueries  map[string]PrometheusQuery = make(map[string]PrometheusQuery)
	templateRatio                                = ``
	templateHistogram                            = ``
	totalQueryTempl                              = template.Must(template.New("").Parse(`
		sum(rate({{.metricId}}{job="{{.jobId}}"}[{{.window}}])) 
	`))

	goodQueryTempl = template.Must(template.New("").Parse(`
		sum(rate({{.metricId}}{job="{{.jobId}}", {{.filter}}}[{{.window}}]))
	`))
)

type RatioQuery struct {
	GoodQuery  string
	TotalQuery string
}

func InitMetricList() {
	availableQueries["uptime"] = NewPrometheusQueryImpl("uptime", "[A-Za-z0-9]_(up){1}", "up=1", true, false)
	availableQueries["http-availability"] = NewPrometheusQueryImpl(
		"http-availibility",
		"[A-Za-z0-9]_(http_request_duration_seconds){1}(.*)",
		"code=~\"(2..|3..)\"", true, false)
	availableQueries["http-latency"] = NewPrometheusQueryImpl(
		"http-latency",
		"le={0.3}",
		"", true, false) //FIXME: fill in
}

type PrometheusQuery interface {
	Name() string
	ConstructRatio(metricId string, jobId string) (*RatioQuery, error)
	IsRatio() bool
	IsHistogram() bool
}

type PrometheusQueryImpl struct {
	name        string
	GoodFilter  string // string expression for filtering promQL queries into good events
	LabelRegex  regexp.Regexp
	isRatio     bool
	isHistogram bool
}

func NewPrometheusQueryImpl(name string, labelRegex string, goodFilter string, isRatio bool, isHistogram bool) *PrometheusQueryImpl {
	return &PrometheusQueryImpl{
		name:        name,
		GoodFilter:  goodFilter,
		LabelRegex:  *regexp.MustCompile(labelRegex),
		isRatio:     isRatio,
		isHistogram: isHistogram,
	}
}

// The actual metricId and window are only known at SLO creation time
func (p *PrometheusQueryImpl) ConstructRatio(jobId string, metricId string) (*RatioQuery, error) {
	var goodQuery bytes.Buffer
	var totalQuery bytes.Buffer
	if err := goodQueryTempl.Execute(&goodQuery, map[string]string{ //NOTE: {{.window}} is filled by sloth generator
		"metricId": metricId,
		"jobId":    jobId,
		"filter":   p.GoodFilter,
		"window":   "{{.window}}",
	}); err != nil {
		return nil, err
	}
	if err := totalQueryTempl.Execute(&totalQuery, map[string]string{ //NOTE: {{.window}} is filled by sloth generator
		"metricId": metricId,
		"jobId":    jobId,
		"window":   "{{.window}}",
	}); err != nil {
		return nil, err
	}

	return &RatioQuery{
		GoodQuery:  strings.TrimSpace(goodQuery.String()),
		TotalQuery: strings.TrimSpace(totalQuery.String()),
	}, nil
}

func (p *PrometheusQueryImpl) Name() string {
	return p.name
}

func (p *PrometheusQueryImpl) ResolveLabel(regex string, availableMetrics []string) string {
	return regex
}

func (p *PrometheusQueryImpl) IsRatio() bool {
	return p.isRatio
}

func (p *PrometheusQueryImpl) IsHistogram() bool {
	return p.isHistogram
}

// @Note: Assumption is that JobID is valid
// @returns goodQuery, totalQuery
func fetchPreconfQueries(slo *api.ServiceLevelObjective, jobId string, metricName string, metricId string, ctx context.Context, lg hclog.Logger) (*RatioQuery, error) {
	if slo.GetDatasource() == MonitoringDatasource {
		if len(availableQueries) == 0 {
			InitMetricList()
		}
		found := false
		for k, _ := range availableQueries {
			if k == metricName {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf(
				"Cannot create SLO with metric name %s ", metricName,
			)
		}
		ratioQuery, err := availableQueries[metricName].ConstructRatio(jobId, metricId)
		if err != nil {
			return nil, err
		}
		return ratioQuery, nil
	} else if slo.GetDatasource() == LoggingDatasource {
		return nil, ErrNotImplemented
	} else {
		return nil, ErrInvalidDatasource
	}
}
