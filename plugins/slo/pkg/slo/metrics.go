package slo

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"math"
	"regexp"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/metrics/unmarshal"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	api "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	availableQueries map[string]PrometheusQuery = make(map[string]PrometheusQuery)
	totalQueryTempl                             = template.Must(template.New("").Parse(`
		sum(rate({{.MetricId}}{job="{{.JobId}}"}[{{"{{.window}}"}}])) 
	`))
	goodQueryTempl = template.Must(template.New("").Parse(`
		sum(rate({{.MetricId}}{job="{{.JobId}}", {{.Filter}}}[{{"{{.window}}"}}]))
	`))
	GetDownstreamMetricQueryTempl = template.Must(template.New("").Parse(`
		group by(__name__)({__name__=~"{{.NameRegex}}"} and {job="{{.ServiceId}}"})
	`))
)

func init() {
	availableQueries["uptime"] = NewPrometheusQueryImpl("uptime",
		".*up.*",
		"up=1", true, false,
		"Measures the uptime of a kubernetes service")
	availableQueries["http-availability"] = NewPrometheusQueryImpl(
		"http-availability",
		".*_http_request_duration_seconds_count",
		"code=~\"(2..|3..)\"", true, false,
		`Measures the availability of a kubernetes service using http status codes. 
		Codes 2XX and 3XX are considered as available.`)
	// TODO : http-latency needs to use buckets in one of the ratios
	availableQueries["http-latency"] = NewPrometheusQueryImpl(
		"http-latency",
		"le={0.3}",
		".*_request_duration_seconds_count", true, false,
		`Quantifiers the latency of http requests made against a kubernetes service
		by classifying them as good (<=300ms) or bad(>=300ms)`)
}

type RatioQuery struct {
	GoodQuery  string
	TotalQuery string
}

type goodQueryPlaceholder struct {
	JobId    string
	MetricId string
	Filter   string
}

type metricTemplate struct {
	NameRegex string
	ServiceId string
}
type PrometheusQuery interface {
	Name() string
	ConstructRatio(service *api.Service) (*RatioQuery, error)
	IsRatio() bool
	IsHistogram() bool
	Description() string
	Datasource() string
	ResolveLabel() *regexp.Regexp
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
func (p *PrometheusQueryImpl) ConstructRatio(service *api.Service) (*RatioQuery, error) {
	var goodQuery bytes.Buffer
	var totalQuery bytes.Buffer
	if err := goodQueryTempl.Execute(&goodQuery,
		goodQueryPlaceholder{MetricId: service.GetMetricId(), JobId: service.GetJobId(), Filter: p.GoodFilter}); err != nil {
		return nil, err
	}
	if err := totalQueryTempl.Execute(
		&totalQuery,
		&service); err != nil {
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

func (p *PrometheusQueryImpl) ResolveLabel() *regexp.Regexp {
	return &p.LabelRegex
}

func (p *PrometheusQueryImpl) IsRatio() bool {
	return p.isRatio
}

func (p *PrometheusQueryImpl) IsHistogram() bool {
	return p.isHistogram
}

func selectBestMatch(metrics []string) string {
	min := math.MaxInt // largest int
	metricId := ""
	for _, m := range metrics {
		if m == "" {
			continue
		}
		if len(m) < min {
			min = len(m)
			metricId = m
		}
	}
	return metricId
}

func assignMetricToJobId(p *Plugin, ctx context.Context, metricRequest *api.MetricRequest) (string, error) {
	lg := p.logger
	var query bytes.Buffer
	if err := GetDownstreamMetricQueryTempl.Execute(&query,
		metricTemplate{
			NameRegex: (availableQueries[metricRequest.GetName()].ResolveLabel()).String(),
			ServiceId: metricRequest.ServiceId,
		},
	); err != nil {
		return "", err
	}
	resp, err := p.adminClient.Get().Query(ctx, &cortexadmin.QueryRequest{
		Tenants: []string{metricRequest.ClusterId},
		Query:   strings.TrimSpace(query.String()),
	})
	if err != nil {
		lg.Error(fmt.Sprintf("Failed to query cluster %v: %v", metricRequest.ClusterId, err))
		return "", err
	}
	res := make([]string, 0)
	data := resp.GetData()
	lg.Debug(fmt.Sprintf("Received service data %s from cluster %s ", string(data), metricRequest.ClusterId))
	q, err := unmarshal.UnmarshallPrometheusResponse(data)
	if err != nil {
		return "", err
	}
	switch q.V.Type() {
	case model.ValVector:
		vv := q.V.(model.Vector)
		if len(vv) == 0 {
			err := status.Error(codes.NotFound,
				fmt.Sprintf("No assignable metric '%s' for service '%s' in cluster '%s' ",
					metricRequest.Name, metricRequest.ServiceId, metricRequest.ClusterId))
			return "", err
		}
		for _, v := range vv {
			res = append(res, string(v.Metric["__name__"]))
		}
		// should always have one + metric
		lg.Debug(fmt.Sprintf("Found metricIds : %v", res))
		metricId := selectBestMatch(res)
		if metricId == "" {
			err := status.Error(codes.NotFound,
				fmt.Sprintf("No assignable metric '%s' for service '%s' in cluster '%s' ",
					metricRequest.Name, metricRequest.ServiceId, metricRequest.ClusterId))
			return "", err
		}
		return metricId, nil
	}
	return "", fmt.Errorf("Could not unmarshall response into expected format")
}

// Note: Assumption is that JobID is valid
// @returns goodQuery, totalQuery
func fetchPreconfQueries(slo *api.ServiceLevelObjective, service *api.Service, ctx context.Context, lg hclog.Logger) (*RatioQuery, error) {
	if slo.GetDatasource() == MonitoringDatasource {
		found := false
		for k := range availableQueries {
			if k == service.GetMetricName() {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf(
				"Cannot create SLO with metric name %s ", service.GetMetricName(),
			)
		}
		ratioQuery, err := availableQueries[service.GetMetricName()].ConstructRatio(service)
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
