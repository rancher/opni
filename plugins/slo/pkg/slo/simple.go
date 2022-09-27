package slo

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"

	promql "github.com/cortexproject/cortex/pkg/configs/legacy_promql"
	"github.com/google/uuid"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v3"
)

const (
	grafana_delete_mask = "slo_opni_delete"
	// id labels
	slo_uuid    = "slo_opni_id"
	slo_service = "slo_opni_service"
	slo_name    = "slo_opni_name"
	slo_window  = "slo_window"

	// recording rule names
	slo_ratio_rate_query_name = "slo:sli_error:ratio_rate"
	slo_alert_ticket_window   = "slo:alert:ticket_window"
	slo_alert_page_window     = "slo:alert:page_window"

	// metadata rule names
	slo_objective_ratio                     = "slo:objective:ratio"
	slo_error_budget_ratio                  = "slo:error_budget:ratio"
	slo_time_period_days                    = "slo:time_period:days"
	slo_current_burn_rate_ratio             = "slo:current_burn_rate:ratio"
	slo_period_burn_rate_ratio              = "slo:period_burn_rate:ratio"
	slo_period_error_budget_remaining_ratio = "slo:period_error_budget_remaining:ratio"
	slo_info                                = "opni_slo_info"

	// alert rule names
	RecordingRuleSuffix = "-recording"
	MetadataRuleSuffix  = "-metadata"
	AlertRuleSuffix     = "-alerts"
)

var (
	simpleQueryTpl = template.Must(template.New("query").Parse(`sum(rate({{.Metric}}{job="{{.JobId}}"{{.Labels}}}[{{.Window}}]))`))
	rawSliQueryTpl = template.Must(template.New("sliRawQuery").Parse(`1 - (({{.GoodQuery}})/({{.TotalQuery}}))`))
	sloFiltersTpl  = template.Must(template.New("sloFilters").Parse(`{{.SloIdLabel}}="{{.SloId}}", {{.SloServiceLabel}}="{{.SloService}}", {{.SloNameLabel}}="{{.SloName}}"`))
)

// SliQueryInfo used for filling sli query templates
type SliQueryInfo struct {
	GoodQuery  string
	TotalQuery string
}

type SloFiltersInfo struct {
	SloIdLabel      string
	SloServiceLabel string
	SloNameLabel    string
	SloId           string
	SloService      string
	SloName         string
}

type RuleGroupYAMLv2 struct {
	Name     string             `yaml:"name"`
	Interval prommodel.Duration `yaml:"interval,omitempty"`
	Rules    []rulefmt.Rule     `yaml:"rules"`
}

type LabelPair struct {
	Key  string
	Vals []string
}

type LabelPairs []LabelPair

func (l LabelPairs) Construct() string { // kinda hacky & technically unoptimized but works
	if len(l) == 0 {
		return ""
	}
	s := ""
	for _, labelPair := range l {
		if labelPair.Key == "" {
			continue
		}
		orCompositionVal := ""
		if len(labelPair.Vals) == 1 {
			orCompositionVal += labelPair.Vals[0]
		} else {
			for idx, val := range labelPair.Vals {
				orCompositionVal += val
				if idx != len(labelPair.Vals)-1 {
					orCompositionVal += "|"
				}
			}
		}
		s += fmt.Sprintf(",%s=~\"%s\"", labelPair.Key, orCompositionVal)
	}
	return s
}

type IdentificationLabels map[string]string
type UserLabels []string
type Service string
type Metric string

type SLO struct {
	sloPeriod   string
	objective   float64 // 0 < x < 100
	svc         Service
	goodMetric  Metric
	totalMetric Metric
	idLabels    IdentificationLabels
	userLabels  map[string]string
	goodEvents  LabelPairs
	totalEvents LabelPairs
}

func normalizeObjective(objective float64) float64 {
	return objective / 100
}

func NewSLO(
	sloName string,
	sloPeriod string,
	objective float64,
	svc Service,
	goodMetric Metric,
	totalMetric Metric,
	userLabels map[string]string,
	goodEvents []LabelPair,
	totalEvents []LabelPair,
) *SLO {
	newId := uuid.New().String()
	ilabels := IdentificationLabels{slo_uuid: newId, slo_name: sloName, slo_service: string(svc)}

	return &SLO{
		svc:         svc,
		sloPeriod:   sloPeriod,
		goodMetric:  goodMetric,
		totalMetric: totalMetric,
		userLabels:  userLabels,
		goodEvents:  goodEvents,
		totalEvents: totalEvents,
		idLabels:    ilabels,
		objective:   objective,
	}
}

func SLOFromId(
	sloName string,
	sloPeriod string,
	objective float64,
	svc Service,
	goodMetric Metric,
	totalMetric Metric,
	userLabels map[string]string,
	goodEvents []LabelPair,
	totalEvents []LabelPair,
	id string,
) *SLO {
	ilabels := IdentificationLabels{slo_uuid: id, slo_name: sloName, slo_service: string(svc)}

	return &SLO{
		svc:         svc,
		goodMetric:  goodMetric,
		totalMetric: totalMetric,
		sloPeriod:   sloPeriod,
		userLabels:  userLabels,
		goodEvents:  goodEvents,
		totalEvents: totalEvents,
		idLabels:    ilabels,
		objective:   objective,
	}
}

func CreateSLORequestToStruct(c *sloapi.CreateSLORequest) *SLO {
	if c.Slo.GetGoodMetricName() == c.Slo.GetTotalMetricName() {
		c.Slo.GoodEvents, c.Slo.TotalEvents = ToMatchingSubsetIdenticalMetric(c.Slo.GoodEvents, c.Slo.TotalEvents)
	}
	reqSLO := c.Slo
	userLabels := reqSLO.GetLabels()
	sloLabels := map[string]string{}
	for _, label := range userLabels {
		sloLabels[label.GetName()] = "true"
	}
	goodEvents := []LabelPair{}
	for _, goodEvent := range reqSLO.GetGoodEvents() {
		goodEvents = append(goodEvents, LabelPair{
			Key:  goodEvent.GetKey(),
			Vals: goodEvent.GetVals(),
		})
	}
	totalEvents := []LabelPair{}
	for _, totalEvent := range reqSLO.GetTotalEvents() {
		totalEvents = append(totalEvents, LabelPair{
			Key:  totalEvent.GetKey(),
			Vals: totalEvent.GetVals(),
		})
	}
	return NewSLO(
		reqSLO.GetName(),
		reqSLO.GetSloPeriod(),
		reqSLO.GetTarget().GetValue(),
		Service(reqSLO.GetServiceId()),
		Metric(reqSLO.GetGoodMetricName()),
		Metric(reqSLO.GetTotalMetricName()),
		sloLabels,
		goodEvents,
		totalEvents,
	)
}

func SLODataToStruct(s *sloapi.SLOData) *SLO {
	reqSLO := s.SLO
	if reqSLO.GetGoodMetricName() == reqSLO.GetTotalMetricName() {
		reqSLO.GoodEvents, reqSLO.TotalEvents = ToMatchingSubsetIdenticalMetric(reqSLO.GoodEvents, reqSLO.TotalEvents)
	}
	userLabels := reqSLO.GetLabels()
	sloLabels := map[string]string{}
	for _, label := range userLabels {
		sloLabels[label.GetName()] = "true"
	}
	goodEvents := []LabelPair{}
	for _, goodEvent := range reqSLO.GetGoodEvents() {
		goodEvents = append(goodEvents, LabelPair{
			Key:  goodEvent.GetKey(),
			Vals: goodEvent.GetVals(),
		})
	}
	totalEvents := []LabelPair{}
	for _, totalEvent := range reqSLO.GetTotalEvents() {
		totalEvents = append(totalEvents, LabelPair{
			Key:  totalEvent.GetKey(),
			Vals: totalEvent.GetVals(),
		})
	}
	if s.Id == "" {
		return NewSLO(
			reqSLO.GetName(),
			reqSLO.GetSloPeriod(),
			reqSLO.GetTarget().GetValue(),
			Service(reqSLO.GetServiceId()),
			Metric(reqSLO.GetGoodMetricName()),
			Metric(reqSLO.GetTotalMetricName()),
			sloLabels,
			goodEvents,
			totalEvents,
		)
	}
	return SLOFromId(
		reqSLO.GetName(),
		reqSLO.GetSloPeriod(),
		reqSLO.GetTarget().GetValue(),
		Service(reqSLO.GetServiceId()),
		Metric(reqSLO.GetGoodMetricName()),
		Metric(reqSLO.GetTotalMetricName()),
		sloLabels,
		goodEvents,
		totalEvents,
		s.Id,
	)
}

func (s *SLO) GetId() string {
	return s.idLabels[slo_uuid] // let it panic if not found
}

func (s *SLO) SetId(id string) {
	s.idLabels[slo_uuid] = id
}

func (s *SLO) GetName() string {
	return s.idLabels[slo_name] // let it panic if not found
}

func (s *SLO) SetName(input string) {
	s.idLabels[slo_name] = input
}

func (s *SLO) GetPeriod() string {
	return s.sloPeriod
}

func (s *SLO) GetObjective() float64 {
	return s.objective
}

func (s *SLO) GetPrometheusRuleFilterByIdLabels() (string, error) {
	var b bytes.Buffer
	err := sloFiltersTpl.Execute(&b, SloFiltersInfo{
		SloIdLabel:      slo_uuid,
		SloServiceLabel: slo_service,
		SloNameLabel:    slo_name,
		SloId:           s.GetId(),
		SloService:      string(s.svc),
		SloName:         s.GetName(),
	})
	return b.String(), err
}

func (s *SLO) RawObjectiveQuery() string {
	return fmt.Sprintf("vector(%.9f)", normalizeObjective(s.objective))
}

func (s *SLO) RawErrorBudgetQuery() string {
	return fmt.Sprintf("vector(1-%.9f)", normalizeObjective(s.objective))
}

// RawCurrentBurnRateQuery
// ratioRate : slo:sli_error:ratio_rate<some-period-string>
func (s *SLO) RawCurrentBurnRateQuery() string {
	ruleFilters, err := s.GetPrometheusRuleFilterByIdLabels()
	if err != nil {
		panic(err)
	}
	fastWindow := NewWindowRange(s.sloPeriod)[0]
	strRes := (slo_ratio_rate_query_name + fastWindow) + "{" + ruleFilters + "}" + " / " +
		fmt.Sprintf("on(%s, %s, %s)", slo_uuid, slo_service, slo_name) + " group_left\n" +
		slo_error_budget_ratio + "{" + ruleFilters + "}"

	return strRes
}

func (s *SLO) RawPeriodBurnRateQuery() string {
	ruleFilters, err := s.GetPrometheusRuleFilterByIdLabels()
	if err != nil {
		panic(err)
	}
	slowestWindow := NewWindowRange(s.sloPeriod)[len(NewWindowRange(s.sloPeriod))-1]
	strRes := (slo_ratio_rate_query_name + slowestWindow) + "{" + ruleFilters + "}" + " / " +
		fmt.Sprintf("on(%s, %s, %s)", slo_uuid, slo_service, slo_name) + " group_left\n" +
		slo_error_budget_ratio + "{" + ruleFilters + "}"
	return strRes
}

func (s *SLO) RawPeriodDurationQuery() string {
	dur, err := prommodel.ParseDuration(s.sloPeriod)
	if err != nil {
		panic(err)
	}
	timeDur := time.Duration(dur)
	days := int(timeDur.Hours() / 24)
	return fmt.Sprintf("vector(%d)", days)
}

func (s *SLO) RawBudgetRemainingQuery() string {
	ruleFilters, err := s.GetPrometheusRuleFilterByIdLabels()
	if err != nil {
		panic(err)
	}
	return "1 - " + slo_period_burn_rate_ratio + "{" + ruleFilters + "}"
}

func (s *SLO) RawDashboardInfoQuery() string {
	return "vector(1)"
}

func (s *SLO) RawGoodEventsQuery(w string) (string, error) {
	goodConstructedEvents := s.goodEvents.Construct()
	var bGood bytes.Buffer
	err := simpleQueryTpl.Execute(&bGood, map[string]string{
		"Metric": string(s.goodMetric),
		"JobId":  string(s.svc),
		"Labels": goodConstructedEvents,
		"Window": w,
	})

	return bGood.String(), err
}

func (s *SLO) RawTotalEventsQuery(w string) (string, error) {
	totalConstructedEvents := s.totalEvents.Construct()
	var bTotal bytes.Buffer
	err := simpleQueryTpl.Execute(&bTotal, map[string]string{
		"Metric": string(s.totalMetric),
		"JobId":  string(s.svc),
		"Labels": totalConstructedEvents,
		"Window": w,
	})
	return bTotal.String(), err
}

func (s *SLO) RawSLIQuery(w string) (string, error) {
	good, err := s.RawGoodEventsQuery(w)
	if err != nil {
		return "", err
	}
	total, err := s.RawTotalEventsQuery(w)
	if err != nil {
		return "", err
	}
	var bQuery bytes.Buffer
	err = rawSliQueryTpl.Execute(&bQuery, SliQueryInfo{
		GoodQuery:  good,
		TotalQuery: total,
	})
	return bQuery.String(), err
}

func (s *SLO) ConstructRecordingRuleGroup(interval *time.Duration) RuleGroupYAMLv2 {
	var promInterval prommodel.Duration
	var err error
	if interval == nil { //sensible default is 1m
		promInterval, err = prommodel.ParseDuration(TimeDurationToPromStr(time.Minute))

	} else {
		promInterval, err = prommodel.ParseDuration(TimeDurationToPromStr(*interval))
		if err != nil {
			panic(err)
		}
	}
	if err != nil {
		panic(err)
	}
	rrecording := RuleGroupYAMLv2{
		Name:     s.GetId() + RecordingRuleSuffix,
		Interval: promInterval,
	}
	for _, w := range NewWindowRange(s.sloPeriod) {
		rawSli, err := s.RawSLIQuery(w)
		if err != nil {
			panic(err)
		}
		rrecording.Rules = append(rrecording.Rules, rulefmt.Rule{
			Record: slo_ratio_rate_query_name + w,
			Expr:   rawSli,
			Labels: MergeLabels(s.idLabels, map[string]string{
				slo_window: w,
			},
				s.userLabels,
			),
		})
	}
	return rrecording
}

func (s *SLO) ConstructMetadataRules(interval *time.Duration) RuleGroupYAMLv2 {
	var promInterval prommodel.Duration
	var err error
	if interval == nil { //sensible default is 1m
		promInterval, err = prommodel.ParseDuration(TimeDurationToPromStr(time.Minute))

	} else {
		promInterval, err = prommodel.ParseDuration(TimeDurationToPromStr(*interval))
		if err != nil {
			panic(err)
		}
	}
	rmetadata := RuleGroupYAMLv2{
		Name:     s.GetId() + MetadataRuleSuffix,
		Interval: promInterval,
	}
	rmetadata.Rules = []rulefmt.Rule{
		{
			Record: slo_objective_ratio,
			Expr:   s.RawObjectiveQuery(),
			Labels: MergeLabels(
				s.idLabels,
				s.userLabels,
			),
		},
		{
			Record: slo_error_budget_ratio,
			Expr:   s.RawErrorBudgetQuery(),
			Labels: MergeLabels(
				s.idLabels,
				s.userLabels,
			),
		},
		{
			Record: slo_time_period_days,
			Expr:   s.RawPeriodDurationQuery(),
			Labels: MergeLabels(
				s.idLabels,
				s.userLabels,
			),
		},
		{
			Record: slo_current_burn_rate_ratio,
			Expr:   s.RawCurrentBurnRateQuery(),
			Labels: MergeLabels(
				s.idLabels,
				s.userLabels,
			),
		},
		{
			Record: slo_period_burn_rate_ratio,
			Expr:   s.RawPeriodBurnRateQuery(),
			Labels: MergeLabels(
				s.idLabels,
				s.userLabels,
			),
		},
		{
			Record: slo_period_error_budget_remaining_ratio,
			Expr:   s.RawBudgetRemainingQuery(),
			Labels: MergeLabels(
				s.idLabels,
				s.userLabels,
			),
		},
		{
			Record: slo_info,
			Expr:   s.RawDashboardInfoQuery(),
			Labels: MergeLabels(
				s.idLabels,
				s.userLabels,
			),
		},
	}
	return rmetadata
}

func (s *SLO) AlertPageThreshold() float64 {
	return 0.5
}

// ConstructAlertingRuleGroup
//
// Note: first two are expected to be the recording rules
// Note: second two are expected to be the alerting rules
func (s *SLO) ConstructAlertingRuleGroup(interval *time.Duration) RuleGroupYAMLv2 {
	var promInterval prommodel.Duration
	var err error
	if interval == nil { //sensible default is 1m
		promInterval, err = prommodel.ParseDuration(TimeDurationToPromStr(time.Minute))

	} else {
		promInterval, err = prommodel.ParseDuration(TimeDurationToPromStr(*interval))
		if err != nil {
			panic(err)
		}
	}
	ralerting := RuleGroupYAMLv2{
		Name:     s.GetId() + AlertRuleSuffix,
		Interval: promInterval,
	}
	sloFilters, err := s.GetPrometheusRuleFilterByIdLabels()
	if err != nil {
		panic(err)
	}
	sloFilters = "{" + sloFilters + "}"
	dur, err := prommodel.ParseDuration(s.sloPeriod)
	if err != nil {
		panic(err)
	}
	mwmbWindow := GenerateGoogleWindows(time.Duration(dur))
	var exprTicket bytes.Buffer
	errorBudgetRatio := 100 - s.objective
	if errorBudgetRatio == 0 {
		//panic(fmt.Sprintf("error budget ratio cannot be treated as 0, from objective : %.9f", s.objective))
	}
	err = mwmbAlertTplBool.Execute(&exprTicket, map[string]string{
		"WindowLabel":          slo_window,
		"QuickShortMetric":     slo_ratio_rate_query_name + "5m",
		"QuickShortBurnFactor": fmt.Sprintf("%.9f", mwmbWindow.GetSpeedTicketQuick()),
		"QuickLongMetric":      slo_ratio_rate_query_name + "30m",
		"QuickLongBurnFactor":  fmt.Sprintf("%.9f", mwmbWindow.GetSpeedTicketSlow()),
		"SlowShortMetric":      slo_ratio_rate_query_name + "2h",
		"SlowShortBurnFactor":  fmt.Sprintf("%.9f", mwmbWindow.GetSpeedTicketQuick()),
		"SlowQuickMetric":      slo_ratio_rate_query_name + "6h",
		"SlowQuickBurnFactor":  fmt.Sprintf("%.9f", mwmbWindow.GetSpeedTicketSlow()),
		"ErrorBudgetRatio":     fmt.Sprintf("%.9f", errorBudgetRatio),
		"MetricFilter":         sloFilters,
	})
	if err != nil {
		panic(err)
	}
	var exprPage bytes.Buffer
	err = mwmbAlertTplBool.Execute(&exprPage, map[string]string{
		"WindowLabel":          slo_window,
		"QuickShortMetric":     slo_ratio_rate_query_name + "5m",
		"QuickShortBurnFactor": fmt.Sprintf("%.9f", mwmbWindow.GetSpeedPageQuick()),
		"QuickLongMetric":      slo_ratio_rate_query_name + "30m",
		"QuickLongBurnFactor":  fmt.Sprintf("%.9f", mwmbWindow.GetSpeedPageSlow()),
		"SlowShortMetric":      slo_ratio_rate_query_name + "2h",
		"SlowShortBurnFactor":  fmt.Sprintf("%.9f", mwmbWindow.GetSpeedPageQuick()),
		"SlowQuickMetric":      slo_ratio_rate_query_name + "6h",
		"SlowQuickBurnFactor":  fmt.Sprintf("%.9f", mwmbWindow.GetSpeedPageSlow()),
		"ErrorBudgetRatio":     fmt.Sprintf("%.9f", errorBudgetRatio),
		"MetricFilter":         sloFilters,
	})
	if err != nil {
		panic(err)
	}
	var recordTicket bytes.Buffer
	err = mwmbAlertTplBool.Execute(&recordTicket, map[string]string{
		"WindowLabel":          slo_window,
		"QuickShortMetric":     slo_ratio_rate_query_name + "5m",
		"QuickShortBurnFactor": fmt.Sprintf("%.9f", mwmbWindow.GetSpeedTicketQuick()),
		"QuickLongMetric":      slo_ratio_rate_query_name + "30m",
		"QuickLongBurnFactor":  fmt.Sprintf("%.9f", mwmbWindow.GetSpeedTicketSlow()),
		"SlowShortMetric":      slo_ratio_rate_query_name + "2h",
		"SlowShortBurnFactor":  fmt.Sprintf("%.9f", mwmbWindow.GetSpeedTicketQuick()),
		"SlowQuickMetric":      slo_ratio_rate_query_name + "6h",
		"SlowQuickBurnFactor":  fmt.Sprintf("%.9f", mwmbWindow.GetSpeedTicketSlow()),
		"ErrorBudgetRatio":     fmt.Sprintf("%.9f", errorBudgetRatio),
		"MetricFilter":         sloFilters,
	})
	if err != nil {
		panic(err)
	}
	var recordPage bytes.Buffer
	err = mwmbAlertTplBool.Execute(&recordPage, map[string]string{
		"WindowLabel":          slo_window,
		"QuickShortMetric":     slo_ratio_rate_query_name + "5m",
		"QuickShortBurnFactor": fmt.Sprintf("%.9f", mwmbWindow.GetSpeedPageQuick()),
		"QuickLongMetric":      slo_ratio_rate_query_name + "30m",
		"QuickLongBurnFactor":  fmt.Sprintf("%.9f", mwmbWindow.GetSpeedPageSlow()),
		"SlowShortMetric":      slo_ratio_rate_query_name + "2h",
		"SlowShortBurnFactor":  fmt.Sprintf("%.9f", mwmbWindow.GetSpeedPageQuick()),
		"SlowQuickMetric":      slo_ratio_rate_query_name + "6h",
		"SlowQuickBurnFactor":  fmt.Sprintf("%.9f", mwmbWindow.GetSpeedPageSlow()),
		"ErrorBudgetRatio":     fmt.Sprintf("%.9f", errorBudgetRatio),
		"MetricFilter":         sloFilters,
	})
	if err != nil {
		panic(err)
	}

	// Note: first two are expected to be the recording rules
	ralerting.Rules = append(ralerting.Rules, rulefmt.Rule{
		Record: slo_alert_ticket_window,
		Expr:   recordTicket.String(),
		Labels: MergeLabels(s.idLabels, map[string]string{"slo_severity": "ticket"}, s.userLabels),
	})
	ralerting.Rules = append(ralerting.Rules, rulefmt.Rule{
		Record: slo_alert_page_window,
		Expr:   recordPage.String(),
		Labels: MergeLabels(s.idLabels, map[string]string{"slo_severity": "page"}, s.userLabels),
	})

	// note: second two are expected to be the alerting rules
	ralerting.Rules = append(ralerting.Rules, rulefmt.Rule{
		Alert:  "slo_alert_page",
		Expr:   exprPage.String(),
		Labels: MergeLabels(s.idLabels, map[string]string{"slo_severity": "page"}, s.userLabels),
	})
	ralerting.Rules = append(ralerting.Rules, rulefmt.Rule{
		Alert:  "slo_alert_ticket",
		Expr:   exprTicket.String(),
		Labels: MergeLabels(s.idLabels, map[string]string{"slo_severity": "ticket"}, s.userLabels),
	})
	return ralerting
}

func (s *SLO) ConstructCortexRules(interval *time.Duration) (sli, metadata, alerts RuleGroupYAMLv2) {
	rrecording := s.ConstructRecordingRuleGroup(interval)
	rmetadata := s.ConstructMetadataRules(interval)
	ralerts := s.ConstructAlertingRuleGroup(interval)

	_, debug := os.LookupEnv("DEBUG_RULES")
	if debug {
		srecording, err := yaml.Marshal(rrecording)
		if err != nil {
			panic(err)
		}
		smetadata, err := yaml.Marshal(rmetadata)
		if err != nil {
			panic(err)
		}
		salerts, err := yaml.Marshal(ralerts)
		if err != nil {
			panic(err)
		}
		os.WriteFile(s.GetId()+"-recording.yaml", srecording, 0644)
		os.WriteFile(s.GetId()+"-metadata.yaml", smetadata, 0644)
		os.WriteFile(s.GetId()+"-alerts.yaml", salerts, 0644)
	}
	return rrecording, rmetadata, ralerts
}

func (s *SLO) ConstructRawAlertQueries() (string, string) {
	filters, err := s.GetPrometheusRuleFilterByIdLabels()
	if err != nil {
		panic(err)
	}
	filters = "{" + filters + "}"
	alertCriticalRawQuery := s.ConstructAlertingRuleGroup(nil).Rules[0].Expr
	alertCriticalRawQuery = strings.Replace(alertCriticalRawQuery, filters, "", -1)
	alertCriticalRawQuery = strings.Replace(alertCriticalRawQuery, fmt.Sprintf("without (%s)", slo_window), "", -1)
	alertSevereRawQuery := s.ConstructAlertingRuleGroup(nil).Rules[1].Expr
	alertSevereRawQuery = strings.Replace(alertSevereRawQuery, filters, "", -1)
	alertSevereRawQuery = strings.Replace(alertSevereRawQuery, fmt.Sprintf("without (%s)", slo_window), "", -1)

	for _, rule := range s.ConstructRecordingRuleGroup(nil).Rules {
		alertCriticalRawQuery = strings.Replace(alertCriticalRawQuery, rule.Record, rule.Expr, -1)
		alertSevereRawQuery = strings.Replace(alertSevereRawQuery, rule.Record, rule.Expr, -1)
	}

	_, err = promql.ParseExpr(alertCriticalRawQuery)
	if err != nil {
		panic(err)
	}
	_, err = promql.ParseExpr(alertSevereRawQuery)
	if err != nil {
		panic(err)
	}
	return alertCriticalRawQuery, alertSevereRawQuery
}

func NewWindowRange(sloPeriod string) []string {
	return []string{"5m", "30m", "1h", "2h", "6h", "1d", sloPeriod}
}

// DetectActiveWindows
//
// @warning Expectation is that the timestamps are ordered when traversing
// matrix --> sample streams --> [] values
// but this may not always be the case
func DetectActiveWindows(severity string, matrix *prommodel.Matrix) ([]*sloapi.AlertFiringWindows, error) {
	returnWindows := []*sloapi.AlertFiringWindows{}
	if matrix == nil {
		return nil, fmt.Errorf("Got empty alerting window %s matrix from cortex", severity)
	}
	for _, row := range *matrix {
		for _, rowValue := range row.Values {
			ts := time.Unix(rowValue.Timestamp.Unix(), 0)
			// rising edge
			if rowValue.Value == 1 {
				if len(returnWindows) == 0 {
					returnWindows = append(returnWindows, &sloapi.AlertFiringWindows{
						Start:    timestamppb.New(ts),
						End:      nil,
						Severity: severity,
					})
				} else {
					if returnWindows[len(returnWindows)-1].End != nil {
						returnWindows = append(returnWindows, &sloapi.AlertFiringWindows{
							Start:    timestamppb.New(ts),
							End:      nil,
							Severity: severity,
						})
					}
				}
			} else { //falling edge
				if len(returnWindows) > 0 && returnWindows[len(returnWindows)-1].End == nil {
					returnWindows[len(returnWindows)-1].End = timestamppb.New(ts)
				}
			}
		}
	}
	if len(returnWindows) > 0 {
		if returnWindows[len(returnWindows)-1].End == nil {
			returnWindows[len(returnWindows)-1].End = timestamppb.New(time.Now())
		}
	}
	return returnWindows, nil
}

// ToMatchingSubsetIdenticalMetric only applies when the good metric & total metric id is the same
func ToMatchingSubsetIdenticalMetric(goodEvents, totalEvents []*sloapi.Event) (good, total []*sloapi.Event) {
	indexGood := map[string][]string{}
	for _, g := range goodEvents {
		indexGood[g.Key] = g.Vals
	}
	for _, t := range totalEvents {
		if t.Key == "" || t.Vals == nil {
			continue
		}
		if _, ok := indexGood[t.Key]; ok { // event type defined on good and total so reconcile
			t.Vals = LeftJoinSlice(indexGood[t.Key], t.Vals)
		} else {
			// event type not defined on good, so coerce good to have it, otherwise
			goodEvents = append(goodEvents, t)
		}
	}
	return goodEvents, totalEvents
}

func MergeRuleGroups(left *RuleGroupYAMLv2, right *RuleGroupYAMLv2) *RuleGroupYAMLv2 {
	newRules := LeftJoinSliceAbstract[rulefmt.Rule, string](
		left.Rules,
		right.Rules,
		func(r rulefmt.Rule) string { return r.Record },
	)
	return &RuleGroupYAMLv2{
		Name:     left.Name,
		Interval: left.Interval,
		Rules:    newRules,
	}
}
