package slo

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"strings"
	"time"

	openslov1 "github.com/alexandreLamarre/oslo/pkg/manifest/v1"
	"github.com/alexandreLamarre/sloth/core/app/generate"
	"github.com/alexandreLamarre/sloth/core/info"
	"github.com/alexandreLamarre/sloth/core/prometheus"
	"github.com/rancher/opni/pkg/slo/shared"
	"gopkg.in/yaml.v2"
)

var errorRatioRawQueryTmpl = template.Must(template.New("").Parse(`
  1 - (
    (
      {{ .good }}
    )
    /
    (
      {{ .total }}
    )
  )
`))

func GeneratePrometheusRule(slos []*prometheus.SLOGroup, ctx context.Context) ([]*generate.Response, error) {
	res := make([]*generate.Response, 0)
	sliRuleGen := prometheus.OptimizedSLIRecordingRulesGenerator
	metaRuleGen := prometheus.MetadataRecordingRulesGenerator
	alertRuleGen := prometheus.SLOAlertRulesGenerator
	controller, err := generate.NewService(generate.ServiceConfig{
		SLIRecordingRulesGenerator:  sliRuleGen,
		MetaRecordingRulesGenerator: metaRuleGen,
		SLOAlertRulesGenerator:      alertRuleGen,
	})
	if err != nil {
		return nil, shared.ErrPrometheusGenerator
	}

	for _, slo := range slos { //FIXME:
		result, err := controller.Generate(ctx, generate.Request{
			ExtraLabels: map[string]string{},
			Info:        info.Info{},
			SLOGroup:    *slo,
		})
		if err != nil {
			return nil, fmt.Errorf("could not generate SLO: %w", err)
		}
		res = append(res, result)
	}
	return res, nil
}

func ParseToPrometheusModel(slos []openslov1.SLO) ([]*prometheus.SLOGroup, error) {
	res := make([]*prometheus.SLOGroup, 0)
	y := NewYAMLSpecLoader(time.Hour * 24 * 30) // FIXME: hardcoded window period
	for idx, slo := range slos {
		m, err := y.MapSpecToModel(slo)
		if err != nil {
			return nil, fmt.Errorf("could not map SLO %d: %w", idx, err)
		}
		res = append(res, m)
	}
	return res, nil
}

type YAMLSpecLoader struct {
	windowPeriod time.Duration
}

// YAMLSpecLoader knows how to load YAML specs and converts them to a model.
func NewYAMLSpecLoader(windowPeriod time.Duration) YAMLSpecLoader {
	return YAMLSpecLoader{
		windowPeriod: windowPeriod,
	}
}

func (YAMLSpecLoader) ValidateTimeWindow(spec openslov1.SLO) error {
	if len(spec.Spec.TimeWindow) == 0 {
		return nil
	}

	if len(spec.Spec.TimeWindow) > 1 {
		return fmt.Errorf("only 1 time window is supported")
	}

	t := spec.Spec.TimeWindow[0]
	if strings.ToLower(t.Duration) != "day" {
		return fmt.Errorf("only days based time windows are supported")
	}

	return nil
}

func (y YAMLSpecLoader) MapSpecToModel(spec openslov1.SLO) (*prometheus.SLOGroup, error) {
	slos, err := y.GetSLOs(spec)
	if err != nil {
		return nil, fmt.Errorf("could not map SLOs correctly: %w", err)
	}
	return &prometheus.SLOGroup{SLOs: slos}, nil
}

func (y YAMLSpecLoader) LoadSpec(ctx context.Context, data []byte) (*prometheus.SLOGroup, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("spec is required")
	}

	s := openslov1.SLO{}
	err := yaml.Unmarshal(data, &s)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshall YAML spec correctly: %w", err)
	}

	// Check version.
	if s.APIVersion != openslov1.APIVersion {
		return nil, fmt.Errorf("invalid spec version, should be %q", openslov1.APIVersion)
	}

	// Check at least we have one SLO.
	if len(s.Spec.Objectives) == 0 {
		return nil, fmt.Errorf("at least one SLO is required")
	}

	// Validate time windows are correct.
	err = y.ValidateTimeWindow(s)
	if err != nil {
		return nil, fmt.Errorf("invalid SLO time windows: %w", err)
	}

	m, err := y.MapSpecToModel(s)
	if err != nil {
		return nil, fmt.Errorf("could not map to model: %w", err)
	}

	return m, nil
}

func (y YAMLSpecLoader) GetSLI(spec openslov1.SLOSpec, slo openslov1.Objective) (*prometheus.SLI, error) {
	if spec.Indicator.Spec.RatioMetric == nil {
		return nil, fmt.Errorf("missing ratioMetrics")
	}

	good := spec.Indicator.Spec.RatioMetric.Good
	total := spec.Indicator.Spec.RatioMetric.Total

	if good.MetricSource.Type != "prometheus" {
		return nil, fmt.Errorf("prometheus source required in spec.Spec.Indicator.Spec.Good")
	}

	if total.MetricSource.Type != "prometheus" {
		return nil, fmt.Errorf("prometheus or sloth query ratio 'total' source is required")
	}

	if good.MetricSource.MetricSourceSpec["queryType"] != "promql" {
		return nil, fmt.Errorf("promql query type required in spec.Spec.Indicator.Spec.Total.MetricSource.MetricSourceSpec")
	}
	if total.MetricSource.MetricSourceSpec["queryType"] != "promql" {
		return nil, fmt.Errorf("promql query type required in spec.Spec.Indicator.Spec.Total.MetricSource.MetricSourceSpec")
	}

	// Map as good and total events as a raw query.
	var b bytes.Buffer
	goodQuery := good.MetricSource.MetricSourceSpec["query"]
	totalQuery := total.MetricSource.MetricSourceSpec["query"]
	err := errorRatioRawQueryTmpl.Execute(&b, map[string]string{"good": goodQuery, "total": totalQuery})
	if err != nil {
		return nil, fmt.Errorf("could not execute mapping SLI template: %w", err)
	}

	return &prometheus.SLI{Raw: &prometheus.SLIRaw{
		ErrorRatioQuery: b.String(),
	}}, nil
}

// getSLOs will try getting all the objectives as individual SLOs, this way we can map
// to what Sloth understands as an SLO, that OpenSLO understands as a list of objectives
// for the same SLO.
func (y YAMLSpecLoader) GetSLOs(spec openslov1.SLO) ([]prometheus.SLO, error) {
	res := []prometheus.SLO{}

	for idx, slo := range spec.Spec.Objectives {
		sli, err := y.GetSLI(spec.Spec, slo)
		if err != nil {
			return nil, fmt.Errorf("could not map SLI: %w", err)
		}

		timeWindow := y.windowPeriod
		if len(spec.Spec.TimeWindow) > 0 {
			timeWindow = time.Duration(30 /*spec.Spec.TimeWindow*/) * 24 * time.Hour // FIXME: convert to time.Duration
		}

		res = append(res, prometheus.SLO{
			ID:              fmt.Sprintf("%s-%s-%d", spec.Spec.Service, spec.Metadata.Name, idx), //FIXME: oslo correct headers
			Name:            fmt.Sprintf("%s-%d", spec.Metadata.Name, idx),                       //FIXME: oslo correct headers
			Service:         spec.Spec.Service,
			Description:     spec.Spec.Description,
			TimeWindow:      timeWindow,
			SLI:             *sli,
			Objective:       slo.Target * 100, // OpenSLO uses ratios, we use percents.
			PageAlertMeta:   prometheus.AlertMeta{Disable: true},
			TicketAlertMeta: prometheus.AlertMeta{Disable: true},
		})
	}

	return res, nil
}
