package cortexadmin

import (
	_ "github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/kralicky/ragu/compat"
)

func init() {
	compat.LoadGogoFileDescriptor("cortex.proto", compat.WithRename("github.com/cortexproject/cortex/pkg/cortexpb/cortex.proto"))
}
