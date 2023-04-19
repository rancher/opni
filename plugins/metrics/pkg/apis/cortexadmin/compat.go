package cortexadmin

import (
	_ "github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/jhump/protoreflect/desc"
	"github.com/kralicky/ragu/compat"
	_ "k8s.io/api/core/v1"
)

func init() {
	compat.LoadGogoFileDescriptor("cortex.proto")
	desc.RegisterImportPath("cortex.proto", "github.com/cortexproject/cortex/pkg/cortexpb/cortex.proto")
}
