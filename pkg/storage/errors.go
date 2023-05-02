package storage

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrNotFound = &NotFoundError{}
var ErrAlreadyExists = &AlreadyExistsError{}

type NotFoundError struct{}

func (e *NotFoundError) Error() string {
	return "not found"
}

func (e *NotFoundError) GRPCStatus() *status.Status {
	return status.New(codes.NotFound, e.Error())
}

type AlreadyExistsError struct{}

func (e *AlreadyExistsError) Error() string {
	return "already exists"
}

func (e *AlreadyExistsError) GRPCStatus() *status.Status {
	return status.New(codes.AlreadyExists, e.Error())
}

func IgnoreErrNotFound(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*NotFoundError); ok {
		return nil
	}
	return err
}
