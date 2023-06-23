package loggingadmin

import (
	"github.com/kralicky/ragu/compat"
	_ "k8s.io/api/core/v1"
)

func init() {
	compat.LoadGogoFileDescriptor("k8s.io/api/core/v1/generated.proto")
	compat.LoadGogoFileDescriptor("k8s.io/apimachinery/pkg/api/resource/generated.proto")
}
