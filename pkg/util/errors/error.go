package errors

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/runtime/protoiface"
)

type GRPCError struct {
	internalError error
	code          codes.Code
}

func New(code codes.Code, inner error) *GRPCError {
	return &GRPCError{
		internalError: inner,
		code:          code,
	}
}

func (e *GRPCError) Error() string {
	return e.internalError.Error()
}

func (e *GRPCError) GRPCStatus() *status.Status {
	type grpcstatus interface{ GRPCStatus() *status.Status }
	if err, ok := e.internalError.(grpcstatus); ok {
		// If the existing status code is OK there won't be any details so
		// we just return the new status
		if err.GRPCStatus().Code() == codes.OK {
			return status.New(e.code, e.internalError.Error())
		}
		newDetails := []protoiface.MessageV1{}
		for _, d := range err.GRPCStatus().Details() {
			if message, ok := d.(protoiface.MessageV1); ok {
				newDetails = append(newDetails, message)
			}
		}
		// Status is already checked above
		newStatus, _ := status.New(e.code, e.internalError.Error()).WithDetails(newDetails...)
		return newStatus
	}
	return status.New(e.code, e.internalError.Error())
}

func (e *GRPCError) Is(target error) bool {
	if targetGRPC, ok := target.(*GRPCError); ok {
		return errors.Is(e.internalError, targetGRPC.internalError)
	}
	return errors.Is(e.internalError, target)
}

func (e *GRPCError) As(target any) bool {
	if targetGRPC, ok := target.(*GRPCError); ok {
		return errors.As(e.internalError, &targetGRPC)
	}
	return errors.As(e.internalError, target)
}
