package slo

import (
	"bytes"
	"fmt"
	"github.com/google/uuid"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"gopkg.in/yaml.v3"
	"os"
	"text/template"
	"time"
)

const (
	// id labels
	slo_uuid    = "slo_opni_id"
	slo_service = "slo_opni_service"
	slo_name    = "slo_opni_name"
	slo_window  = "slo_window"

	// recording rule names
	slo_ratio_rate_query_name = "slo:sli_error:ratio_rate"

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
		if len(labelPair.Vals) == 1 {
			s += fmt.Sprintf(",%s=\"%s\"", labelPair.Key, labelPair.Vals[0])
		} else {
			orCompositionVal := ""
			for idx, val := range labelPair.Vals {
				orCompositionVal += val
				if idx != len(labelPair.Vals)-1 {
					orCompositionVal += "|"
				}
			}
			s += fmt.Sprintf(",%s=~\"%s\"", labelPair.Key, orCompositionVal)
		}
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
	userLabels  UserLabels
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
	userLabels UserLabels,
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
	userLabels UserLabels,
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

func (s *SLO) GetId() string {
	return s.idLabels[slo_uuid] // let it panic if not found
}

func (s *SLO) GetName() string {
	return s.idLabels[slo_name] // let it panic if not found
}

func (s *SLO) GetPeriod() string {
	return s.sloPeriod
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
	return fmt.Sprintf("vector(%f)", normalizeObjective(s.objective))
}

func (s *SLO) RawErrorBudgetQuery() string {
	return fmt.Sprintf("vector(1-%f)", normalizeObjective(s.objective))
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
	//TODO implement
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
			}),
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
		},
		{
			Record: slo_error_budget_ratio,
			Expr:   s.RawErrorBudgetQuery(),
		},
		{
			Record: slo_time_period_days,
			Expr:   s.RawPeriodDurationQuery(),
		},
		{
			Record: slo_current_burn_rate_ratio,
			Expr:   s.RawCurrentBurnRateQuery(),
		},
		{
			Record: slo_period_burn_rate_ratio,
			Expr:   s.RawPeriodBurnRateQuery(),
		},
		{
			Record: slo_period_error_budget_remaining_ratio,
			Expr:   s.RawBudgetRemainingQuery(),
		},
		{
			Record: slo_info,
			Expr:   s.RawDashboardInfoQuery(),
		},
	}
	return rmetadata
}

func (s *SLO) AlertPageThreshold() float64 {
	return 0.5
}

func execAlertTemplate(b *bytes.Buffer, ruleName, ruleFilters, floatThreshold, sloWindowLabel string) error {
	maxCalcTpl := template.Must(template.New("alertingMaxExprCal").Parse("max({{.RuleName}}{ {{.RuleFilters}}} > {{.FloatThreshold}}) without({{.SloWindowLabel}})"))
	return maxCalcTpl.Execute(b, map[string]string{
		"RuleName":       ruleName,
		"RuleFilters":    ruleFilters,
		"FloatThreshold": floatThreshold,
		"SloWindowLabel": sloWindowLabel,
	})
}

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
	var pageConditionFast1, pageConditionFast2, pageConditionSlow1, pageConditionSlow2 bytes.Buffer
	if err := execAlertTemplate(&pageConditionFast1, slo_ratio_rate_query_name+"5m", sloFilters, "0.0001", slo_window); err != nil {
		panic(err)
	}
	if err := execAlertTemplate(&pageConditionFast2, slo_ratio_rate_query_name+"30m", sloFilters, "0.0001", slo_window); err != nil {
		panic(err)
	}
	if err := execAlertTemplate(&pageConditionSlow1, slo_ratio_rate_query_name+"2h", sloFilters, "0.0001", slo_window); err != nil {
		panic(err)
	}
	if err := execAlertTemplate(&pageConditionSlow2, slo_ratio_rate_query_name+"6h", sloFilters, "0.0001", slo_window); err != nil {
		panic(err)
	}
	ralerting.Rules = append(ralerting.Rules, rulefmt.Rule{
		Alert: fmt.Sprintf("%s_alert_page-%s", s.GetId(), s.GetName()),
		Expr: fmt.Sprintf("(%s and %s) or (%s and %s)",
			pageConditionFast1.String(),
			pageConditionFast2.String(),
			pageConditionSlow1.String(),
			pageConditionSlow2.String(),
		),
		Labels: MergeLabels(s.idLabels, map[string]string{"slo_severity": "page"}),
	})

	var ticketConditionFast1, ticketConditionFast2, ticketConditionSlow1, ticketConditionSlow2 bytes.Buffer

	if err := execAlertTemplate(&ticketConditionFast1, slo_ratio_rate_query_name+"2h", sloFilters, "0.0001", slo_window); err != nil {
		panic(err)
	}
	if err := execAlertTemplate(&ticketConditionFast2, slo_ratio_rate_query_name+"1d", sloFilters, "0.0001", slo_window); err != nil {
		panic(err)
	}
	if err := execAlertTemplate(&ticketConditionSlow1, slo_ratio_rate_query_name+"6h", sloFilters, "0.0001", slo_window); err != nil {
		panic(err)
	}
	if err := execAlertTemplate(&ticketConditionSlow2, slo_ratio_rate_query_name+"3d", sloFilters, "0.0001", slo_window); err != nil {
		panic(err)
	}
	ralerting.Rules = append(ralerting.Rules, rulefmt.Rule{
		Alert: fmt.Sprintf("%s_alert_ticket-%s", s.GetId(), s.GetName()),
		Expr: fmt.Sprintf("(%s and %s) or (%s and %s)",
			ticketConditionFast1.String(),
			ticketConditionFast2.String(),
			ticketConditionSlow1.String(),
			ticketConditionSlow2.String(),
		),
		Labels: MergeLabels(s.idLabels, map[string]string{"slo_severity": "page"}),
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

func NewWindowRange(sloPeriod string) []string {
	return []string{"5m", "30m", "1h", "2h", "6h", "1d", sloPeriod}
}
