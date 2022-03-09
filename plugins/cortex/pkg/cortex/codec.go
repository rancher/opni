package cortex

import "github.com/rancher/opni-monitoring/pkg/util"

var (
	orgIDCodec = util.NewDelimiterCodec("X-Scope-OrgID", "|")
)
