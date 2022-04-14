package cortex

import "github.com/rancher/opni/pkg/util"

var (
	orgIDCodec = util.NewDelimiterCodec("X-Scope-OrgID", "|")
)
