package cortexadmin

import (
	_ "github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/jhump/protoreflect/desc"
	"github.com/kralicky/ragu/compat"
)

func init() {
	compat.LoadGogoFileDescriptor("cortex.proto")
	desc.RegisterImportPath("cortex.proto", "github.com/cortexproject/cortex/pkg/cortexpb/cortex.proto")
}
