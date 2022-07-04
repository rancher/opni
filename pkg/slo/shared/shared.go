package shared

/*
Shared Enum, consts and errors for SLO apis & packages
*/

import (
	"fmt"

	validation "github.com/rancher/opni/pkg/validation"
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

	OsloVersion = "openslo/v1"
)
