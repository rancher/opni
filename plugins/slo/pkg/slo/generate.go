package slo

// This file is for generating SLOs without the builtin sloth OO generator
// which crashes

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"sort"
	"strconv"
	"time"

	"github.com/alexandreLamarre/sloth/core/alert"
	"github.com/alexandreLamarre/sloth/core/info"
	"github.com/alexandreLamarre/sloth/core/prometheus"
	"github.com/hashicorp/go-hclog"
	"github.com/prometheus/prometheus/model/rulefmt"
)

const (
	// Metrics.
	sliErrorMetricFmt = "slo:sli_error:ratio_rate%s"

	// Labels.
	sloNameLabelName      = "sloth_slo"
	sloIDLabelName        = "sloth_id"
	sloServiceLabelName   = "sloth_service"
	sloWindowLabelName    = "sloth_window"
	sloSeverityLabelName  = "sloth_severity"
	sloVersionLabelName   = "sloth_version"
	sloModeLabelName      = "sloth_mode"
	sloSpecLabelName      = "sloth_spec"
	sloObjectiveLabelName = "sloth_objective"
)

type ruleFmtWrapper struct {
	SLIrules   []rulefmt.Rule
	MetaRules  []rulefmt.Rule
	AlertRules []rulefmt.Rule
}

func Validate(slos *prometheus.SLOGroup) error {
	// TODO implement
	// use prometheus field validators
	return nil
}

func GenerateMWWBAlerts(ctx context.Context, alertSLO alert.SLO, timeWindow time.Duration) (*alert.MWMBAlertGroup, error) {
	windows := WindowDefaults(timeWindow)
	errorBudget := 100 - alertSLO.Objective
	group := alert.MWMBAlertGroup{
		PageQuick: alert.MWMBAlert{
			ID:             fmt.Sprintf("%s-page-quick", alertSLO.ID),
			ShortWindow:    windows.PageQuick.ShortWindow,
			LongWindow:     windows.PageQuick.LongWindow,
			BurnRateFactor: windows.GetSpeedPageQuick(),
			ErrorBudget:    errorBudget,
			Severity:       alert.PageAlertSeverity,
		},
		PageSlow: alert.MWMBAlert{
			ID:             fmt.Sprintf("%s-page-slow", alertSLO.ID),
			ShortWindow:    windows.PageSlow.ShortWindow,
			LongWindow:     windows.PageSlow.LongWindow,
			BurnRateFactor: windows.GetSpeedPageSlow(),
			ErrorBudget:    errorBudget,
			Severity:       alert.PageAlertSeverity,
		},
		TicketQuick: alert.MWMBAlert{
			ID:             fmt.Sprintf("%s-ticket-quick", alertSLO.ID),
			ShortWindow:    windows.TicketQuick.ShortWindow,
			LongWindow:     windows.TicketQuick.LongWindow,
			BurnRateFactor: windows.GetSpeedTicketQuick(),
			ErrorBudget:    errorBudget,
			Severity:       alert.TicketAlertSeverity,
		},
		TicketSlow: alert.MWMBAlert{
			ID:             fmt.Sprintf("%s-ticket-slow", alertSLO.ID),
			ShortWindow:    windows.TicketSlow.ShortWindow,
			LongWindow:     windows.TicketSlow.LongWindow,
			BurnRateFactor: windows.GetSpeedTicketSlow(),
			ErrorBudget:    errorBudget,
			Severity:       alert.TicketAlertSeverity,
		},
	}
	return &group, nil
}

func GenerateSLIRecordingRules(ctx context.Context, slo prometheus.SLO, alerts alert.MWMBAlertGroup) ([]rulefmt.Rule, error) {
	// Get the windows we need the recording rules.
	windows := getAlertGroupWindows(alerts)
	windows = append(windows, slo.TimeWindow) // Add the total time window as a handy helper.

	// Generate the rules
	rules := make([]rulefmt.Rule, 0, len(windows))
	for _, window := range windows {
		rule, err := rawSLIRecordGenerator(slo, window, alerts) //s.genFunc(slo, window, alerts)
		if err != nil {
			return nil, fmt.Errorf("could not create %q SLO rule for window %s: %w", slo.ID, window, err)
		}
		rules = append(rules, *rule)
	}

	return rules, nil
}

// getAlertGroupWindows gets all the time windows from a multiwindow multiburn alert group.
func getAlertGroupWindows(alerts alert.MWMBAlertGroup) []time.Duration {
	// Use a map to avoid duplicated windows.
	windows := map[string]time.Duration{
		alerts.PageQuick.ShortWindow.String():   alerts.PageQuick.ShortWindow,
		alerts.PageQuick.LongWindow.String():    alerts.PageQuick.LongWindow,
		alerts.PageSlow.ShortWindow.String():    alerts.PageSlow.ShortWindow,
		alerts.PageSlow.LongWindow.String():     alerts.PageSlow.LongWindow,
		alerts.TicketQuick.ShortWindow.String(): alerts.TicketQuick.ShortWindow,
		alerts.TicketQuick.LongWindow.String():  alerts.TicketQuick.LongWindow,
		alerts.TicketSlow.ShortWindow.String():  alerts.TicketSlow.ShortWindow,
		alerts.TicketSlow.LongWindow.String():   alerts.TicketSlow.LongWindow,
	}

	res := make([]time.Duration, 0, len(windows))
	for _, w := range windows {
		res = append(res, w)
	}
	sort.SliceStable(res, func(i, j int) bool { return res[i] < res[j] })

	return res
}

func rawSLIRecordGenerator(slo prometheus.SLO, window time.Duration, alerts alert.MWMBAlertGroup) (*rulefmt.Rule, error) {
	// Render with our templated data.
	sliExprTpl := fmt.Sprintf(`(%s)`, slo.SLI.Raw.ErrorRatioQuery)
	tpl, err := template.New("sliExpr").Option("missingkey=error").Parse(sliExprTpl)
	if err != nil {
		return nil, fmt.Errorf("could not create SLI expression template data: %w", err)
	}

	strWindow := timeDurationToPromStr(window)
	var b bytes.Buffer
	err = tpl.Execute(&b, map[string]string{
		tplKeyWindow: strWindow,
	})
	if err != nil {
		return nil, fmt.Errorf("could not render SLI expression template: %w", err)
	}

	return &rulefmt.Rule{
		Record: slo.GetSLIErrorMetric(window),
		Expr:   b.String(),
		Labels: MergeLabels(
			slo.GetSLOIDPromLabels(),
			map[string]string{
				sloWindowLabelName: strWindow,
			},
			slo.Labels,
		),
	}, nil
}

func GenerateMetadataRecordingRules(ctx context.Context, slo prometheus.SLO, alerts *alert.MWMBAlertGroup) ([]rulefmt.Rule, error) {
	labels := MergeLabels(slo.GetSLOIDPromLabels(), slo.Labels)

	// Metatada Recordings.
	const (
		metricSLOObjectiveRatio                  = "slo:objective:ratio"
		metricSLOErrorBudgetRatio                = "slo:error_budget:ratio"
		metricSLOTimePeriodDays                  = "slo:time_period:days"
		metricSLOCurrentBurnRateRatio            = "slo:current_burn_rate:ratio"
		metricSLOPeriodBurnRateRatio             = "slo:period_burn_rate:ratio"
		metricSLOPeriodErrorBudgetRemainingRatio = "slo:period_error_budget_remaining:ratio"
		metricSLOInfo                            = "sloth_slo_info"
	)

	sloObjectiveRatio := slo.Objective / 100

	sloFilter := labelsToPromFilter(slo.GetSLOIDPromLabels())

	var currentBurnRateExpr bytes.Buffer
	err := burnRateRecordingExprTpl.Execute(&currentBurnRateExpr, map[string]string{
		"SLIErrorMetric":         slo.GetSLIErrorMetric(alerts.PageQuick.ShortWindow),
		"MetricFilter":           sloFilter,
		"SLOIDName":              sloIDLabelName,
		"SLOLabelName":           sloNameLabelName,
		"SLOServiceName":         sloServiceLabelName,
		"ErrorBudgetRatioMetric": metricSLOErrorBudgetRatio,
	})
	if err != nil {
		return nil, fmt.Errorf("could not render current burn rate prometheus metadata recording rule expression: %w", err)
	}

	var periodBurnRateExpr bytes.Buffer
	err = burnRateRecordingExprTpl.Execute(&periodBurnRateExpr, map[string]string{
		"SLIErrorMetric":         slo.GetSLIErrorMetric(slo.TimeWindow),
		"MetricFilter":           sloFilter,
		"SLOIDName":              sloIDLabelName,
		"SLOLabelName":           sloNameLabelName,
		"SLOServiceName":         sloServiceLabelName,
		"ErrorBudgetRatioMetric": metricSLOErrorBudgetRatio,
	})
	if err != nil {
		return nil, fmt.Errorf("could not render period burn rate prometheus metadata recording rule expression: %w", err)
	}

	rules := []rulefmt.Rule{
		// SLO Objective.
		{
			Record: metricSLOObjectiveRatio,
			Expr:   fmt.Sprintf(`vector(%g)`, sloObjectiveRatio),
			Labels: labels,
		},

		// Error budget.
		{
			Record: metricSLOErrorBudgetRatio,
			Expr:   fmt.Sprintf(`vector(1-%g)`, sloObjectiveRatio),
			Labels: labels,
		},

		// Total period.
		{
			Record: metricSLOTimePeriodDays,
			Expr:   fmt.Sprintf(`vector(%g)`, slo.TimeWindow.Hours()/24),
			Labels: labels,
		},

		// Current burning speed.
		{
			Record: metricSLOCurrentBurnRateRatio,
			Expr:   currentBurnRateExpr.String(),
			Labels: labels,
		},

		// Total period burn rate.
		{
			Record: metricSLOPeriodBurnRateRatio,
			Expr:   periodBurnRateExpr.String(),
			Labels: labels,
		},

		// Total Error budget remaining period.
		{
			Record: metricSLOPeriodErrorBudgetRemainingRatio,
			Expr:   fmt.Sprintf(`1 - %s%s`, metricSLOPeriodBurnRateRatio, sloFilter),
			Labels: labels,
		},

		// Info.
		{
			Record: metricSLOInfo,
			Expr:   `vector(1)`,
			Labels: MergeLabels(labels, map[string]string{
				sloVersionLabelName:   info.Version,
				sloObjectiveLabelName: strconv.FormatFloat(slo.Objective, 'f', -1, 64),
			}),
		},
	}
	return rules, nil
}

func GenerateSLOAlertRules(ctx context.Context, slo prometheus.SLO, alerts alert.MWMBAlertGroup) ([]rulefmt.Rule, error) {
	// TODO implement
	return nil, nil
}

func GenerateSLO(slo prometheus.SLO, ctx context.Context, info info.Info, lg hclog.Logger) (*ruleFmtWrapper, error) {

	// Generate with the MWWB alerts

	alertSLO := alert.SLO{
		ID:        slo.ID,
		Objective: slo.Objective,
	}
	as, err := GenerateMWWBAlerts(ctx, alertSLO, slo.TimeWindow)
	if err != nil {
		return nil, fmt.Errorf("Could not generate SLO alerts: %w", err)
	}
	lg.Info("Multiwindow-multiburn alerts generated")

	sliRecordingRules, err := GenerateSLIRecordingRules(ctx, slo, *as)
	if err != nil {
		return nil, fmt.Errorf("Could not generate SLO recording rules: %w", err)
	}

	metaRecordingRules, err := GenerateMetadataRecordingRules(ctx, slo, as)
	if err != nil {
		return nil, fmt.Errorf("Could not generate metadata recording rules %w", err)
	}

	alertRules, err := GenerateSLOAlertRules(ctx, slo, *as)
	if err != nil {
		return nil, fmt.Errorf("Could not generate SLO alert rules: %w", err)
	}

	return &ruleFmtWrapper{
		SLIrules:   sliRecordingRules,
		MetaRules:  metaRecordingRules,
		AlertRules: alertRules,
	}, nil
}

func GenerateNoSloth(slos *prometheus.SLOGroup, ctx context.Context, lg hclog.Logger) ([][]byte, error) {
	res := make([][]byte, 0)
	err := Validate(slos)
	if err != nil {
		return nil, err
	}

	for _, slo := range slos.SLOs {
		extraLabels := map[string]string{}
		slo.Labels = MergeLabels(slo.Labels, extraLabels)
		i := info.Info{}

		result, err := GenerateSLO(slo, ctx, i, lg)
		if err != nil {
			return nil, err
		}
		lg.Info(fmt.Sprintf("SLO generated: %+v", result))
		// res = append(res, result)
	}
	return res, nil
}
