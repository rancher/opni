package storage

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrNotFound = &NotFoundError{}

type NotFoundError struct{}

func (e *NotFoundError) Error() string {
	return "not found"
}

func (e *NotFoundError) GRPCStatus() *status.Status {
	return status.New(codes.NotFound, e.Error())
}
