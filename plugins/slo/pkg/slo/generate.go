package slo

// This file is for generating SLOs without the builtin sloth OO generator
// which crashes

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"sort"
	"time"

	"github.com/alexandreLamarre/sloth/core/alert"
	"github.com/alexandreLamarre/sloth/core/info"
	"github.com/alexandreLamarre/sloth/core/prometheus"
	"github.com/hashicorp/go-hclog"
	prommodel "github.com/prometheus/common/model"
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

func MergeLabels(ms ...map[string]string) map[string]string {
	res := map[string]string{}
	for _, m := range ms {
		for k, v := range m {
			res[k] = v
		}
	}
	return res
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

const (
	tplKeyWindow = "window"
)

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

// Pretty simple durations for prometheus.
func timeDurationToPromStr(t time.Duration) string {
	return prommodel.Duration(t).String()
}

func GenerateMetadataRecordingRules(ctx context.Context, slo prometheus.SLO, alerts *alert.MWMBAlertGroup) ([]rulefmt.Rule, error) {
	// TODO implement
	return nil, nil
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
