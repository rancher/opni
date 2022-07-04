package slo

import (
	"github.com/rancher/opni/pkg/slo/shared"
	api "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
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
		return shared.ErrInvalidId
	}
	if err := validateSLODescription(slo.GetDescription()); err != nil {
		return err
	}
	if err := validateSLODatasource(slo.GetDatasource()); err != nil {
		return err
	}
	if err := validateAlert(slo.GetAlerts()); err != nil {
		return err
	}

	return nil
}

func validateAlert(alerts []*api.Alert) error {
	for _, alert := range alerts {
		if err := matchEnum(alert.GetNotificationTarget(), []string{
			shared.NotifSlack, shared.NotifMail, shared.NotifPager, shared.NotifHook}, shared.ErrInvalidAlertTarget); err != nil {
			return err
		}
		if err := matchEnum(alert.GetConditionType(), []string{shared.AlertingBurnRate, shared.AlertingBudget, shared.AlertingTarget}, shared.ErrInvalidAlertCondition); err != nil {
			return err
		}
		if err := matchEnum(alert.GetThresholdType(), []string{shared.GTThresholdType, shared.LTThresholdType}, shared.ErrInvalidAlertThreshold); err != nil {
			return err
		}
	}
	return nil
}

func validateSLODatasource(value string) error {
	return matchEnum(value, []string{shared.LoggingDatasource, shared.MonitoringDatasource}, shared.ErrInvalidDatasource)
}

func validateSLODescription(value string) error {
	if len(value) > 1050 {
		return shared.ErrInvalidDescription
	}
	return nil
}
