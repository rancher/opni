package storage

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrNotFound = status.Error(codes.NotFound, "not found")
var ErrAlreadyExists = status.Error(codes.AlreadyExists, "already exists")
var ErrConflict = status.Error(codes.Aborted, "conflict")
