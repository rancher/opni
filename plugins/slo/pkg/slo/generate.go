package slo

// This file is for generating SLOs without the builtin sloth OO generator
// which crashes

import (
	"context"
	"fmt"
	"time"

	"github.com/alexandreLamarre/sloth/core/alert"
	"github.com/alexandreLamarre/sloth/core/info"
	"github.com/alexandreLamarre/sloth/core/prometheus"
	"github.com/hashicorp/go-hclog"
	"github.com/prometheus/prometheus/model/rulefmt"
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

func MergeLabels(sloLabel map[string]string, extraLabels map[string]string) map[string]string {
	// TODO implement
	return sloLabel
}

func GenerateMWWBAlerts(ctx context.Context, alertSLO alert.SLO, timeWindow time.Duration) (*alert.MWMBAlertGroup, error) {
	//TODO decouple this nightmare of filesystem repository builders from sloth
	window, err := alert.NewFSWindowsRepo(alert.FSWindowsRepoConfig{})
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize window")
	}
	windows, err := window.GetWindows(ctx, timeWindow)
	if err != nil {
		return nil, fmt.Errorf("Failed to get windows: %w", err)
	}

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

func GenerateSLIRecordingRUles(ctx context.Context, slo prometheus.SLO, alerts *alert.MWMBAlertGroup) ([]rulefmt.Rule, error) {
	// TODO implement
	return nil, nil
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

	sliRecordingRules, err := GenerateSLIRecordingRUles(ctx, slo, as)
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
