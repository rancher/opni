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
	GetDownstreamMetricQueryTempl = template.Must(template.New("").Parse(`
		group by(__name__)({__name__="{{.Name}}" and {job="{{.serviceId}}"}})
	`))
)

type RatioQuery struct {
	GoodQuery  string
	TotalQuery string
}

func InitMetricList() {
	availableQueries["uptime"] = NewPrometheusQueryImpl("uptime",
		"[A-Za-z0-9]_(up){1}",
		"up=1", true, false,
		"Measures the uptime of a kubernetes service")
	availableQueries["http-availability"] = NewPrometheusQueryImpl(
		"http-availibility",
		"[A-Za-z0-9]_(http_request_duration_seconds){1}(.*)",
		"code=~\"(2..|3..)\"", true, false,
		`Measures the availability of a kubernetes service using http status codes. 
		Codes 2XX and 3XX are considered as available.`)
	availableQueries["http-latency"] = NewPrometheusQueryImpl(
		"http-latency",
		"le={0.3}",
		"[A-Za-z0-9]_(request_duration_seconds_count){1}(.*)", true, false,
		`Quantifiers the latency of http requests made against a kubernetes service
		by classifying them as good (<=300ms) or bad(>=300ms)`)
}

type PrometheusQuery interface {
	Name() string
	ConstructRatio(metricId string, jobId string) (*RatioQuery, error)
	IsRatio() bool
	IsHistogram() bool
	Description() string
	Datasource() string
}

type PrometheusQueryImpl struct {
	name        string
	GoodFilter  string // string expression for filtering promQL queries into good events
	LabelRegex  regexp.Regexp
	isRatio     bool
	isHistogram bool
	datasource  string
	description string
}

func NewPrometheusQueryImpl(name string, labelRegex string, goodFilter string, isRatio bool,
	isHistogram bool, description string) *PrometheusQueryImpl {
	return &PrometheusQueryImpl{
		name:        name,
		datasource:  MonitoringDatasource,
		description: description,
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

func (p *PrometheusQueryImpl) Datasource() string {
	return p.datasource
}

func (p *PrometheusQueryImpl) Description() string {
	return p.description
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
