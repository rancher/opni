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
	slo_uuid              = "slo_opni_id"
	slo_service           = "slo_opni_service"
	slo_name              = "slo_opni_name"
	ratio_rate_query_name = "slo:sli_error:ratio_rate"
	RecordingRuleSuffix   = "-recording"
	MetadataRuleSuffix    = "-metadata"
	AlertRuleSuffix       = "-alerts"
)

var (
	goodQueryTpl = template.Must(template.New("").Parse(`
	   sum(rate({{.Metric}}\{job=\"{{.JobId }}\"{{.Labels}}\}[{{.Window}}]))	
	`))
	totalQueryTpl = template.Must(template.New("").Parse(`
		sum(rate({{.Metric}}\{job=\"{{.JobId}}\"{{.Labels}}\}[{{"{{.Window}}"}}]))
	`))
	rawSliQueryTpl = template.Must(template.New("").Parse(`
		1 - (({{.GoodQuery}})/({{.TotalQuery}}))
	`))
)

// QueryInfo used for filling query Templates
type QueryInfo struct {
	Metric string
	JodId  string
	Labels string
	Window string
}

// SliQueryInfo used for filling sli query templates
type SliQueryInfo struct {
	GoodQuery  string
	TotalQuery string
}

type ruleGroupYAMLv2 struct {
	Name     string             `yaml:"name"`
	Interval prommodel.Duration `yaml:"interval,omitempty"`
	Rules    []rulefmt.Rule     `yaml:"rules"`
}

type LabelPair struct {
	Key string
	Val string
}

type LabelPairs []LabelPair

func (l LabelPairs) Construct() string {
	if len(l) == 0 {
		return ""
	}
	s := ""
	for _, labelpair := range l {
		s += fmt.Sprintf(",%s=\"%s\"", labelpair.Key, labelpair.Val)
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

func NewSLO(
	sloName string,
	sloPeriod string,
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
	}
}

func SLOFromId(
	sloName string,
	goodMetric Metric,
	totalMetric Metric,
	sloPeriod string,
	svc Service,
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
	}
}

func (s *SLO) GetId() string {
	return s.idLabels[slo_uuid] // let it panic if not found
}

func (s *SLO) GetName() string {
	return s.idLabels[slo_name] // let it panic if not found
}

func (s *SLO) RawSLIQuery(w string) (string, error) {
	goodConstructedEvents := s.goodEvents.Construct()
	var bGood bytes.Buffer
	q := QueryInfo{
		Metric: string(s.goodMetric),
		JodId:  string(s.svc),
		Labels: goodConstructedEvents,
		Window: w,
	}
	err := goodQueryTpl.Execute(&bGood, q)
	if err != nil {
		return "", err
	}
	var bTotal bytes.Buffer
	err = totalQueryTpl.Execute(&bTotal, QueryInfo{
		Metric: string(s.totalMetric),
		JodId:  string(s.svc),
		Labels: goodConstructedEvents,
		Window: w,
	})
	if err != nil {
		return "", err
	}
	var bQuery bytes.Buffer
	err = rawSliQueryTpl.Execute(&bQuery, SliQueryInfo{
		GoodQuery:  bGood.String(),
		TotalQuery: bTotal.String(),
	})
	return bQuery.String(), err
}

func (s *SLO) ConstructCortexRules() (queryStr string) {
	interval, err := prommodel.ParseDuration(timeDurationToPromStr(time.Second))
	if err != nil {
		panic(err)
	}
	rrecording := ruleGroupYAMLv2{
		Name:     s.GetId() + RecordingRuleSuffix,
		Interval: interval,
	}
	rmetadata := ruleGroupYAMLv2{
		Name:     s.GetId() + RecordingRuleSuffix,
		Interval: interval,
	}
	ralerts := ruleGroupYAMLv2{
		Name:     s.GetId() + RecordingRuleSuffix,
		Interval: interval,
	}
	for _, w := range NewWindowRange(s.sloPeriod) {
		rawSli, err := s.RawSLIQuery(w)
		if err != nil {
			panic(err)
		}
		rrecording.Rules = append(rrecording.Rules, rulefmt.Rule{
			Record: ratio_rate_query_name + w,
			Expr:   rawSli,
			Labels: MergeLabels(s.idLabels, map[string]string{
				"slo_window": w,
			}),
		})
	}

	rmetadata.Rules = []rulefmt.Rule{
		{
			Record: "slo:objective:ratio",
			Expr:   fmt.Sprintf("vector(0.%f)", s.objective/100),
		},
		{
			Record: "slo:error_budget:ratio",
			//TODO
		},
		{
			Record: "slo:time_period:days",
			//TODO
		},
		{
			Record: "slo:current_burn_rate:ratio",
			//TODO
		},
		{
			Record: "slo:period_burn_rate:ratio",
			//TODO
		},
		{
			Record: "slo:period_error_budget_remaining:ratio",
			//TODO
		},
		{
			Record: "sloth_slo_info",
			//TODO
		},
	}

	ralerts.Rules = []rulefmt.Rule{
		{
			Alert: fmt.Sprintf("%s-alert-page-%s", s.GetId(), s.GetName()),
			//TODO
			//	expr: |
			//(
			//	max(slo:sli_error:ratio_rate5m{sloth_id="test-slo-0", sloth_service="MyServer", sloth_slo="test-slo-0"} > (0.0016666666666666666 * 0.00010000000000005117)) without (sloth_window)
			//	and
			//	max(slo:sli_error:ratio_rate1h{sloth_id="test-slo-0", sloth_service="MyServer", sloth_slo="test-slo-0"} > (0.0016666666666666666 * 0.00010000000000005117)) without (sloth_window)
			//)
			//	or
			//(
			//	max(slo:sli_error:ratio_rate30m{sloth_id="test-slo-0", sloth_service="MyServer", sloth_slo="test-slo-0"} > (0.0006944444444444445 * 0.00010000000000005117)) without (sloth_window)
			//	and
			//	max(slo:sli_error:ratio_rate6h{sloth_id="test-slo-0", sloth_service="MyServer", sloth_slo="test-slo-0"} > (0.0006944444444444445 * 0.00010000000000005117)) without (sloth_window)
			//)
			//	labels:
			//	sloth_severity: page
		},
		{
			Alert: fmt.Sprintf("%s-alert-ticket-%s", s.GetId(), s.GetName()),
			//	expr: |
			//(
			//	max(slo:sli_error:ratio_rate2h{sloth_id="test-slo-0", sloth_service="MyServer", sloth_slo="test-slo-0"} > (0.00034722222222222224 * 0.00010000000000005117)) without (sloth_window)
			//	and
			//	max(slo:sli_error:ratio_rate1d{sloth_id="test-slo-0", sloth_service="MyServer", sloth_slo="test-slo-0"} > (0.00034722222222222224 * 0.00010000000000005117)) without (sloth_window)
			//)
			//	or
			//(
			//	max(slo:sli_error:ratio_rate6h{sloth_id="test-slo-0", sloth_service="MyServer", sloth_slo="test-slo-0"} > (0.00011574074074074075 * 0.00010000000000005117)) without (sloth_window)
			//	and
			//	max(slo:sli_error:ratio_rate3d{sloth_id="test-slo-0", sloth_service="MyServer", sloth_slo="test-slo-0"} > (0.00011574074074074075 * 0.00010000000000005117)) without (sloth_window)
			//	labels:
			//	sloth_severity: ticket
		},
	}
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
	return queryStr
}

func NewWindowRange(sloPeriod string) []string {
	return []string{"5m", "30m", "1h", "2h", "6h", "1d", sloPeriod}
}
