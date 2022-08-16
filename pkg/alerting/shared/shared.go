/*
Shared definitions (constants & errors) for opni alerting
*/
package shared

import (
	"fmt"

	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	AlertingV1Alpha      = "v1alpha"
	MonitoringDatasource = "monitoring"
	LoggingDatasource    = "logging"
)

var (
	AlertingErrNotImplemented           = WithUnimplementedError("Not implemented")
	AlertingErrNotImplementedNOOP       = WithUnimplementedError("Alerting NOOP : Not implemented")
	AlertingErrParseBucket              = WithInternalServerError("Failed to parse bucket index")
	AlertingErrBucketIndexInvalid       = WithInternalServerError("Bucket index is invalid")
	AlertingErrInvalidSlackChannel      = validation.Error("Slack channel invalid : must start with '#'")
	AlertingErrK8sRuntime               = WithInternalServerError("K8s Runtime error")
	AlertingErrMismatchedImplementation = validation.Error("Alerting endpoint did not match the given implementation")
)

type UnimplementedError struct {
	message string
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
	return &InternalServerError{
		message: fmt.Errorf(format, args...).Error(),
	}
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
