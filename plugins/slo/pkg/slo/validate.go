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
	ErrInvalidAlertCondition = validation.Error("Invalid Alert Condition")
	ErrInvalidAlertTarget    = validation.Error("Invalid Alert Target")
	ErrInvalidId             = validation.Error("Internal error, unassigned SLO ID")
	ErrNotImplemented        = validation.Error("Not implemented")
)

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
	for _, alert := range slo.Alerts {
		if err := validateSLOAlertCondition(alert.GetConditionType()); err != nil {
			return err
		}
		if err := validateSLOAlertTarget(alert.GetNotificationTarget()); err != nil {
			return err
		}
	}

	return nil
}

func validateSLOAlertTarget(value string) error {
	// TODO : validate alert target
	switch value {
	case NotifMail:
		return nil
	case NotifSlack:
		return nil
	case NotifPager:
		return nil
	default:
		return ErrInvalidAlertTarget
	}
}

func validateSLOAlertCondition(value string) error {
	// TODO : validate alert condition
	switch value {
	case AlertingBudget:
		return nil
	case AlertingBurnRate:
		return nil
	default:
		return ErrInvalidAlertCondition
	}
}

func validateSLOMetric(value string) error {
	// TODO : validate metric type with grouping

	return nil
}

func validateSLODatasource(value string) error {
	switch value {
	case LoggingDatasource:
		return nil
	case MonitoringDatasource:
		return nil
	default:
		return ErrInvalidDatasource
	}
}

func validateSLODescription(value string) error {
	if len(value) > 1050 {
		return ErrInvalidDescription
	}
	return nil
}
