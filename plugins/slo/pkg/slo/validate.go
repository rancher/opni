package slo

import (
	"fmt"

	validation "github.com/rancher/opni/pkg/validation"
	api "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
)

// validation error descriptions
var (
	lengthConstraint         = func(i int) string { return fmt.Sprintf("must be between 1-%d characters in length", i) }
	ErrInvalidDescription    = validation.Errorf("Description %s", lengthConstraint(1050))
	ErrInvalidDatasource     = validation.Error("Invalid required datasource value")
	ErrInvalidMetric         = validation.Error("Invalid preconfigured metric")
	ErrInvalidAlertType      = validation.Error("Invalid alert type")
	ErrInvalidAlertCondition = validation.Error("Invalid Alert Condition")
	ErrInvalidAlertThreshold = validation.Error("Invalid Alert Threshold")
	ErrInvalidAlertTarget    = validation.Error("Invalid Alert Target")
	ErrInvalidId             = validation.Error("Internal error, unassigned SLO ID")
	ErrNotImplemented        = validation.Error("Not implemented")
	ErrPrometheusGenerator   = validation.Error("Prometheus generator failed to start")
)

const (
	// Metric Enum
	MetricLatency      = "http-latency"
	MetricAvailability = "http-availability"

	// Datasource Enum
	LoggingDatasource    = "logging"
	MonitoringDatasource = "monitoring"

	// Alert Enum
	AlertingBurnRate = "burnrate"
	AlertingBudget   = "budget"
	AlertingTarget   = "target"

	GTThresholdType = ">"
	LTThresholdType = "<"

	// Notification Enum
	NotifSlack = "slack"
	NotifMail  = "email"
	NotifPager = "pager"
	NotifHook  = "webhook"

	osloVersion = "openslo/v1"
)

func matchEnum(target string, enum []string, returnErr error) error {
	for _, v := range enum {
		if v == target {
			return nil
		}
	}
	return returnErr
}

/// Validates Input based on the necessities of our preconfigured formant,
/// NOT validating the OpenSLO / Sloth format
func ValidateInput(slo *api.ServiceLevelObjective) error {
	if slo.GetId() == "" {
		return ErrInvalidId
	}
	if err := validateSLODescription(slo.GetDescription()); err != nil {
		return err
	}
	if err := validateSLODatasource(slo.GetDatasource()); err != nil {
		return err
	}
	if err := validateSLOMetric("Placeholder"); err != nil { //FIXME:
		return err
	}
	if err := validateAlert(slo.GetAlerts()); err != nil {
		return err
	}

	return nil
}

func validateAlert(alerts []*api.Alert) error {
	for _, alert := range alerts {
		if err := matchEnum(alert.GetNotificationTarget(), []string{NotifSlack, NotifMail, NotifPager, NotifHook}, ErrInvalidAlertTarget); err != nil {
			return err
		}
		if err := matchEnum(alert.GetConditionType(), []string{AlertingBurnRate, AlertingBudget, AlertingTarget}, ErrInvalidAlertCondition); err != nil {
			return err
		}
		if err := matchEnum(alert.GetThresholdType(), []string{GTThresholdType, LTThresholdType}, ErrInvalidAlertThreshold); err != nil {
			return err
		}
	}
	return nil
}

func validateSLOMetric(value string) error {
	// TODO : validate metric type when metric grouping works
	return nil
}

func validateSLODatasource(value string) error {
	return matchEnum(value, []string{LoggingDatasource, MonitoringDatasource}, ErrInvalidDatasource)
}

func validateSLODescription(value string) error {
	if len(value) > 1050 {
		return ErrInvalidDescription
	}
	return nil
}
