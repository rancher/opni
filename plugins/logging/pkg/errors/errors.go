package errors

import (
	"errors"
	"fmt"

	utilerrors "github.com/rancher/opni/pkg/util/errors"
	"google.golang.org/grpc/codes"
)

var (
	ErrInvalidList             = utilerrors.New(codes.FailedPrecondition, errors.New("list did not return exactly 1 result"))
	ErrInvalidPersistence      = utilerrors.New(codes.InvalidArgument, errors.New("invalid persistence config"))
	ErrInvalidDataPersistence  = utilerrors.New(codes.InvalidArgument, errors.New("minimum of 2 data nodes required if no persistent storage"))
	ErrInvalidUpgradeOptions   = utilerrors.New(codes.InvalidArgument, errors.New("upgrade options not valid with current cluster config"))
	ErrClusterIDMissing        = utilerrors.New(codes.InvalidArgument, errors.New("request does not include cluster ID"))
	ErrOpensearchResponse      = utilerrors.New(codes.Unavailable, errors.New("opensearch request unsuccessful"))
	ErrNoOpensearchClient      = utilerrors.New(codes.Unavailable, errors.New("opensearch client is not set"))
	ErrLoggingCapabilityExists = utilerrors.New(codes.FailedPrecondition, errors.New("at least one cluster has logging capability installed"))
	ErrInvalidDuration         = utilerrors.New(codes.InvalidArgument, errors.New("duration must be integer and time unit, e.g 7d"))
	ErrRequestMissingMemory    = utilerrors.New(codes.InvalidArgument, errors.New("memory limit must be configured"))
	ErrMissingDataNode         = utilerrors.New(codes.InvalidArgument, errors.New("request must contain data nodes"))
	ErrRequestGtLimits         = utilerrors.New(codes.InvalidArgument, errors.New("cpu requests must not be more than cpu limits"))
	ErrReplicasZero            = utilerrors.New(codes.InvalidArgument, errors.New("replicas must be a positive nonzero integer"))
	ErrAlreadyExists           = utilerrors.New(codes.AlreadyExists, errors.New("object with that name already exists"))
	ErrObjectNotFound          = utilerrors.New(codes.NotFound, errors.New("object not found"))
)

func WrappedGetPrereqFailed(inner error) *utilerrors.GRPCError {
	return utilerrors.New(codes.FailedPrecondition, fmt.Errorf("failed to get required object: %w", inner))
}

func WrappedOpensearchFailure(inner error) *utilerrors.GRPCError {
	return utilerrors.New(codes.Internal, fmt.Errorf("error communicating with opensearch: %w", inner))
}
