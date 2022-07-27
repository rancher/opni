package shared

/*
Shared Enum, consts and errors for SLO apis & packages
*/

import (
	"fmt"

	validation "github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// validation error descriptions
var (
	lengthConstraint            = func(i int) string { return fmt.Sprintf("must be between 1-%d characters in length", i) }
	ErrMissingServices          = validation.Error("Service definitions are required to create an SLO")
	ErrInvalidMonitorWindow     = validation.Error("Invalid `monitorWindow` in the body of the request")
	ErrInvalidTarget            = validation.Error("Invalid `target` in the body of the request")
	ErrInvalidBudgetingInterval = validation.Error("Invalid `budgetingInterval` in the body of the request : Must be between 1m & 1h")
	ErrInvalidMetricName        = validation.Error("Invalid `metricName` in the body of the request")
	ErrInvalidDescription       = validation.Errorf("Description %s", lengthConstraint(1050))
	ErrInvalidMetric            = validation.Error("Invalid preconfigured metric")
	ErrInvalidAlertType         = validation.Error("Invalid alert type")
	ErrInvalidAlertCondition    = validation.Error("Invalid Alert Condition")
	ErrInvalidAlertThreshold    = validation.Error("Invalid Alert Threshold")
	ErrInvalidAlertTarget       = validation.Error("Invalid Alert Target")
	ErrMissingServiceInfo       = validation.Error("Service job & cluster id are required to create an SLO")
	ErrNonNullId                = WithInternalServerError("Slo Create requests must not have a set id")
	ErrPrometheusGenerator      = WithInternalServerError("Prometheus generator failed to start")
	ErrInvalidDatasource        = WithNotFoundError("Invalid required datasource value")
	ErrNotImplemented           = WithUnimplementedError("Not Implemented")
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

type NotFoundError struct {
	message string
}

func (e *NotFoundError) Error() string {
	return e.message
}

func (e *NotFoundError) GRPCStatus() *status.Status {
	return status.New(codes.NotFound, e.message)
}

func WithNotFoundError(msg string) error {
	return &NotFoundError{
		message: msg,
	}
}

func WithNotFoundErrorf(format string, args ...interface{}) error {
	return &NotFoundError{
		message: fmt.Errorf(format, args...).Error(),
	}
}

type UnimplementedError struct {
	message string
}

func (e *UnimplementedError) Error() string {
	return e.message
}

func (e *UnimplementedError) GRPCStatus() *status.Status {
	return status.New(codes.Unimplemented, e.message)
}

func WithUnimplementedError(msg string) error {
	return &UnimplementedError{
		message: msg,
	}
}

func WithUnimplementedErrorf(format string, args ...interface{}) error {
	return &UnimplementedError{
		message: fmt.Errorf(format, args...).Error(),
	}
}

type InternalServerError struct {
	message string
}

func (e *InternalServerError) Error() string {
	return e.message
}

func (e *InternalServerError) GRPCStatus() *status.Status {
	return status.New(codes.Internal, e.message)
}

func WithInternalServerError(msg string) error {
	return &InternalServerError{
		message: msg,
	}
}

func WithInternalServerErrorf(format string, args ...interface{}) error {
	return &UnimplementedError{
		message: fmt.Errorf(format, args...).Error(),
	}
}
