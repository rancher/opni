package capabilities

import (
	"errors"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

var ErrUnknownCapability = errors.New("unknown capability")

func Has[T corev1.MetadataAccessor[U], U corev1.Capability[U]](
	accessor T,
	capability U,
) bool {
	for _, cap := range accessor.GetCapabilities() {
		if cap.Equal(capability) {
			return true
		}
	}
	return false
}
