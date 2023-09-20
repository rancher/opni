package k8sutil

import (
	"github.com/rancher/opni/pkg/util/errors"
	"google.golang.org/grpc/codes"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GRPCFromK8s(err error) *errors.GRPCError {
	switch k8serrors.ReasonForError(err) {
	case metav1.StatusReasonNotFound, metav1.StatusReasonGone:
		return errors.New(codes.NotFound, err)
	case metav1.StatusReasonUnauthorized:
		return errors.New(codes.Unauthenticated, err)
	case metav1.StatusReasonForbidden:
		return errors.New(codes.PermissionDenied, err)
	case metav1.StatusReasonAlreadyExists:
		return errors.New(codes.AlreadyExists, err)
	case metav1.StatusReasonInvalid:
		return errors.New(codes.InvalidArgument, err)
	case metav1.StatusReasonServerTimeout, metav1.StatusReasonTooManyRequests:
		return errors.New(codes.ResourceExhausted, err)
	case metav1.StatusReasonTimeout:
		return errors.New(codes.DeadlineExceeded, err)
	case metav1.StatusReasonBadRequest, metav1.StatusReasonMethodNotAllowed:
		return errors.New(codes.FailedPrecondition, err)
	case metav1.StatusReasonNotAcceptable, metav1.StatusReasonUnsupportedMediaType:
		return errors.New(codes.Unimplemented, err)
	case metav1.StatusReasonInternalError:
		return errors.New(codes.Internal, err)
	case metav1.StatusReasonExpired, metav1.StatusReasonConflict:
		return errors.New(codes.Aborted, err)
	case metav1.StatusReasonServiceUnavailable:
		return errors.New(codes.Unavailable, err)
	default:
		return errors.New(codes.Unknown, err)
	}
}
