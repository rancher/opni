package query

/*
This module defines the datatypes & interfaces used to define SLO queries.
`AvailableQueries` contains the list of all preconfigured queries.

Queries used by SLOs must follow a format like :

totalQueryTempl = template.Must(template.New("").Parse(`
		sum(rate({{.MetricId}}{job="{{.JobId}}"}[{{"{{.window}}"}}]))
	`))
goodQueryTempl = template.Must(template.New("").Parse(`
	sum(rate({{.MetricId}}{job="{{.JobId}}", {{.Filter}}}[{{"{{.window}}"}}]))
`))

Must :

1. Include a nested template with a {{.window}} for SLOs to fill in
2. the templates must only include information that can be filled with the `templateExecutor` struct.
   Note: templates are intended to be filled with *api.Service protobuf definitions, so expect only that information will
   be available, when SLOs are created at runtime
*/

import (
	"net/http"
	"regexp"
	"strings"
	"text/template"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/opni/pkg/slo/shared"
	api "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
)

var (
	AvailableQueries              map[string]MetricQuery = make(map[string]MetricQuery)
	GetDownstreamMetricQueryTempl                        = template.Must(template.New("").Parse(`
		group by(__name__)({__name__=~"{{.NameRegex}}"})
	`))
)

type templateExecutor struct {
	MetricIdGood  string
	MetricIdTotal string
	JobId         string
}

// must not contain spaces
const MockTestServerName = "MyServer"

func init() {
	// Names should be unique for each pre-configured query, as they are used as keys
	// in the map

	//FIXME: doesn't turn into a prometheus range
	uptimeSLOQuery := New().
		Name("uptime").
		GoodQuery(
			NewQueryBuilder().
				Query(`
					(sum(rate({{.MetricIdGood}}{job="{{.JobId}}"} == 1)))[{{"{{.window}}"}}]
				`).
				MetricFilter(`up`).
				BuildRatio()).
		TotalQuery(
			NewQueryBuilder().
				Query(`
					(sum(rate({{.MetricIdTotal}}{job="{{.JobId}}"})))[{{"{{.window}}"}}]
				`).
				MetricFilter(`up`).BuildRatio()).
		Collector(uptimeCollector).
		GoodEventGenerator(uptimeGoodEvents).
		BadEventGenerator(uptimeBadEvents).
		Description("Measures the uptime of a kubernetes service").
		Datasource(shared.MonitoringDatasource).Build()
	AvailableQueries[uptimeSLOQuery.name] = &uptimeSLOQuery

	httpAvailabilitySLOQuery := New().
		Name("http-availability").
		GoodQuery(
			NewQueryBuilder().
				Query(`
					(sum(rate({{.MetricIdGood}}{job="{{.JobId}}",code=~"(2..|3..)"}[{{"{{.window}}"}}])))
				`).
				MetricFilter(`.*http_request_duration_seconds_count`).
				BuildRatio()).
		TotalQuery(
			NewQueryBuilder().
				Query(`
				(sum(rate({{.MetricIdTotal}}{job="{{.JobId}}"}[{{"{{.window}}"}}])))
				`).
				MetricFilter(`.*http_request_duration_seconds_count`).BuildRatio()).
		Collector(availabilityCollector).
		GoodEventGenerator(availabilityGoodEvents).
		BadEventGenerator(availabilityBadEvents).
		Description(`Measures the availability of a kubernetes service using http status codes.
		Codes 2XX and 3XX are considered as available.`).
		Datasource(shared.MonitoringDatasource).Build()
	AvailableQueries[httpAvailabilitySLOQuery.name] = &httpAvailabilitySLOQuery

	httpResponseTimeSLOQuery := New().
		Name("http-latency").
		GoodQuery(
			NewQueryBuilder().
				Query(`
					sum(rate({{.MetricIdGood}}{job="{{.JobId}},"le="0.3",verb!="WATCH"}[{{"{{.window}}"}}]))
				`).
				MetricFilter(`.*http_request_duration_seconds_bucket`).
				BuildHistogram()).
		TotalQuery(
			NewQueryBuilder().
				Query(`
					sum(rate({{.MetricIdTotal}}{job="{{.JobId}}",verb!="WATCH"}[{{"{{.window}}"}}]))
				`).
				MetricFilter(`.*http_request_duration_seconds_count`).BuildRatio()).
		Collector(latencyCollector).
		GoodEventGenerator(latencyGoodEvents).
		BadEventGenerator(latencyBadEvents).
		Description(`Quantifies the latency of http requests made against a kubernetes service
			by classifying them as good (<=300ms) or bad(>=300ms)`).
		Datasource(shared.MonitoringDatasource).Build()
	AvailableQueries[httpResponseTimeSLOQuery.name] = &httpResponseTimeSLOQuery
}

type matcher func([]string) string

type SLOQueryResult struct {
	GoodQuery  string
	TotalQuery string
}

type MetricQuery interface {
	// User facing name of the pre-confured metric
	Name() string
	// User-facing description of the pre-configured metric
	Description() string
	// Each metric has a unique opni datasource (monitoring vs logging) by which it is filtered by
	Datasource() string
	Construct(*api.ServiceInfo) (*SLOQueryResult, error)
	// Some metrics will have different labels for metrics, so handle them independently
	GetGoodQuery() Query
	GetTotalQuery() Query

	// Auto-instrumentation server methods

	GetCollector() prometheus.Collector
	GetGoodEventGenerator() func(w http.ResponseWriter, r *http.Request)
	GetBadEventGenerator() func(w http.ResponseWriter, r *http.Request)
}

type Query interface {
	FillQueryTemplate(info templateExecutor) (string, error)
	GetMetricFilter() string
	Validate() error
	IsRatio() bool
	BestMatch([]string) string
	IsHistogram() bool
	Construct(*api.ServiceInfo) (string, error)
}

type QueryBuilder interface {
	Query(string) QueryBuilder
	MetricFilter(string) QueryBuilder
	Matcher(*matcher) QueryBuilder
	BuildRatio() RatioQuery
	BuildHistogram() HistogramQuery
}

type queryBuilder struct {
	query   template.Template
	filter  regexp.Regexp
	matcher matcher
}

func NewQueryBuilder() QueryBuilder {
	return queryBuilder{}
}

func (q queryBuilder) Query(query string) QueryBuilder {
	tmpl := template.Must(template.New("").Parse(strings.TrimSpace(query)))
	q.query = *tmpl
	return q
}

func (q queryBuilder) MetricFilter(filter string) QueryBuilder {
	regex := regexp.MustCompile(strings.TrimSpace(filter))
	q.filter = *regex
	return q
}

// defaults to `MatchMinLength`
func (q queryBuilder) Matcher(matcher *matcher) QueryBuilder {
	if matcher == nil {
		q.matcher = MatchMinLength
	} else {
		q.matcher = *matcher
	}
	return q
}

func (q queryBuilder) BuildRatio() RatioQuery {
	if q.matcher == nil {
		q.matcher = MatchMinLength
	}
	r := RatioQuery{
		query:        q.query,
		metricFilter: q.filter,
		matcher:      q.matcher,
	}
	err := r.Validate()
	if err != nil {
		panic(err)
	}
	return r
}

func (q queryBuilder) BuildHistogram() HistogramQuery {
	if q.matcher == nil {
		q.matcher = MatchMinLength
	}
	h := HistogramQuery{
		query:        q.query,
		metricFilter: q.filter,
		matcher:      q.matcher,
	}
	err := h.Validate()
	if err != nil {
		panic(err)
	}
	return h
}

type SloQueryBuilder interface {
	Name(name string) SloQueryBuilder
	GoodQuery(q Query) SloQueryBuilder
	TotalQuery(q Query) SloQueryBuilder
	Description(description string) SloQueryBuilder
	Datasource(datasource string) SloQueryBuilder
	Collector(prometheus.Collector) SloQueryBuilder
	GoodEventGenerator(goodEvents func(w http.ResponseWriter, r *http.Request)) SloQueryBuilder
	BadEventGenerator(badEvents func(w http.ResponseWriter, r *http.Request)) SloQueryBuilder
	Build() PrometheusQueryImpl
}

type sloQueryBuilder struct {
	name           string
	metricFilter   regexp.Regexp
	goodQuery      Query
	totalQuery     Query
	description    string
	datasource     string
	collector      prometheus.Collector
	totalCollector prometheus.Collector
	goodEventsGen  func(w http.ResponseWriter, r *http.Request)
	badEventsGen   func(w http.ResponseWriter, r *http.Request)
}

func New() SloQueryBuilder {
	return sloQueryBuilder{}
}

func (s sloQueryBuilder) Name(name string) SloQueryBuilder {
	s.name = name
	return s
}

func (s sloQueryBuilder) GoodQuery(q Query) SloQueryBuilder {
	s.goodQuery = q
	return s
}

func (s sloQueryBuilder) TotalQuery(q Query) SloQueryBuilder {
	s.totalQuery = q
	return s
}

func (s sloQueryBuilder) Description(description string) SloQueryBuilder {
	s.description = description
	return s
}

func (s sloQueryBuilder) Datasource(datasource string) SloQueryBuilder {
	s.datasource = datasource
	return s
}

func (s sloQueryBuilder) Collector(
	collector prometheus.Collector) SloQueryBuilder {
	s.collector = collector
	return s
}

func (s sloQueryBuilder) GoodEventGenerator(
	goodEvents func(w http.ResponseWriter, r *http.Request),
) SloQueryBuilder {
	s.goodEventsGen = goodEvents
	return s
}

func (s sloQueryBuilder) BadEventGenerator(
	badEvents func(w http.ResponseWriter, r *http.Request),
) SloQueryBuilder {
	s.badEventsGen = badEvents
	return s
}

func (s sloQueryBuilder) Build() PrometheusQueryImpl {
	//TODO figure out how to validate at compile time
	if s.collector == nil {
		panic("auto-instrumentation collectors must be set")
	}
	if s.goodEventsGen == nil || s.badEventsGen == nil {
		panic("auto-instrumentation events must be set")
	}
	return PrometheusQueryImpl{
		name:          s.name,
		datasource:    s.datasource,
		description:   s.description,
		GoodQuery:     s.goodQuery,
		TotalQuery:    s.totalQuery,
		collector:     s.collector,
		goodEventsGen: s.goodEventsGen,
		badEventsGen:  s.badEventsGen,
	}
}
