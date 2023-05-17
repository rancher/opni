package kubernetes

import (
	"errors"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	ErrImageNotFound        = errors.New("image not found in status")
	ErrUnsupportedImageType = errors.New("unsupported image type")
)

func retriableError(err error) bool {
	return k8serrors.IsNotFound(err) || errors.Is(err, ErrImageNotFound)
}
