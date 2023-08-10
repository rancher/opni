package storage

import (
	"github.com/samber/lo"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/runtime/protoimpl"
)

var ErrNotFound = status.Error(codes.NotFound, "not found")
var ErrAlreadyExists = status.Error(codes.AlreadyExists, "already exists")
var ErrConflict = lo.Must(status.New(codes.Aborted, "conflict").WithDetails(ErrDetailsConflict)).Err()

// Use this instead of errors.Is(err, ErrNotFound). The implementation of Is()
// for grpc status errors compares the error message, which can result in false negatives.
func IsNotFound(err error) bool {
	return status.Code(err) == codes.NotFound
}

// Use this instead of errors.Is(err, ErrAlreadyExists). The implementation of Is()
// for grpc status errors compares the error message, which can result in false negatives.
func IsAlreadyExists(err error) bool {
	return status.Code(err) == codes.AlreadyExists
}

// Use this instead of errors.Is(err, ErrConflict). The status code is too
// generic to identify conflict errors, so there are additional details added
// to conflict errors to disambiguate them.
func IsConflict(err error) bool {
	stat := status.Convert(err)
	if stat.Code() != codes.Aborted {
		return false
	}
	for _, detail := range stat.Details() {
		if proto.Equal(protoimpl.X.ProtoMessageV2Of(detail), ErrDetailsConflict) {
			return true
		}
	}
	return false
}

var ErrDetailsConflict = &errdetails.ErrorInfo{Reason: "CONFLICT"}
