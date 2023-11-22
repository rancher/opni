package cortex

import "github.com/rancher/opni/pkg/util"

var (
	OrgIDCodec = util.NewDelimiterCodec("X-Scope-OrgID", "|")
)
