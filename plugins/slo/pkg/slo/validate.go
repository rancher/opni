package slo

import (
	"golang.org/x/exp/slices"

	"github.com/rancher/opni/pkg/slo/query"
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

// Validates Input based on the necessities of our preconfigured formant,
// NOT validating the OpenSLO / Sloth format
func ValidateInput(slorequest *api.CreateSLORequest) error {
	if len(slorequest.Services) == 0 {
		return shared.ErrMissingServices
	}
	if err := validateSLODatasource(slorequest); err != nil {
		return err
	}
	if err := validateSLOWindows(slorequest); err != nil {
		return err
	}
	if err := validateSLOBudget(slorequest); err != nil {
		return err
	}
	if err := validateTarget(slorequest); err != nil {
		return err
	}
	if err := validateAlert(slorequest); err != nil {
		return err
	}
	if err := validateMetricName(slorequest); err != nil {
		return err
	}

	for _, svc := range slorequest.Services {
		if svc.JobId == "" || svc.ClusterId == "" {
			return shared.ErrMissingServiceInfo
		}
	}

	return nil
}

func validateAlert(slorequest *api.CreateSLORequest) error {
	if slorequest.SLO.GetAlerts() == nil {
		return nil
	}
	alerts := slorequest.SLO.GetAlerts()
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

func validateSLOWindows(slorequest *api.CreateSLORequest) error {
	validWindows := []string{"7d", "28d", "30d"}
	if !slices.Contains(validWindows, slorequest.SLO.GetMonitorWindow()) {
		return shared.ErrInvalidMonitorWindow
	}
	return nil
}

func validateSLOBudget(slorequest *api.CreateSLORequest) error {
	budgetTime := slorequest.SLO.GetBudgetingInterval()
	if budgetTime == nil {
		return shared.ErrInvalidBudgetingInterval
	}
	if !(budgetTime.Seconds >= 60 && budgetTime.Seconds <= 60*60) {
		return shared.ErrInvalidBudgetingInterval
	}
	return nil
}

func validateSLODatasource(slorequest *api.CreateSLORequest) error {
	value := slorequest.SLO.GetDatasource()
	return matchEnum(value, []string{shared.LoggingDatasource, shared.MonitoringDatasource}, shared.ErrInvalidDatasource)
}

func validateSLODescription(value string) error {
	if len(value) > 1050 {
		return shared.ErrInvalidDescription
	}
	return nil
}

func validateTarget(slorequest *api.CreateSLORequest) error {
	target := slorequest.SLO.GetTarget()
	if target == nil {
		return shared.ErrInvalidTarget
	}
	if target.GetValue() >= 100 || target.GetValue() <= 0 {

	}
	return nil
}

func validateMetricName(slorequest *api.CreateSLORequest) error {
	metricName := slorequest.SLO.GetMetricName()
	if metricName == "" {
		return shared.ErrInvalidMetricName
	}
	if _, ok := query.AvailableQueries[metricName]; !ok {
		return shared.ErrInvalidMetricName
	}
	return nil
}
