/*
Shared definitions (constants & errors) for opni alerting
*/
package shared

import (
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	AlertingV1Alpha = "v1alpha"
)

var (
	AlertingErrNotImplemented     = WithUnimplementedError("Not implemented")
	AlertingErrNotImplementedNOOP = WithUnimplementedError("Alerting NOOP : Not implemented")
)

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
