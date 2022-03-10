package capabilities

import "github.com/rancher/opni-monitoring/pkg/core"

type TokenCapabilities string

const (
	JoinExistingCluster TokenCapabilities = "join_existing_cluster"
)

func (tc TokenCapabilities) For(ref *core.Reference) *core.TokenCapability {
	return &core.TokenCapability{
		Type:      string(tc),
		Reference: ref,
	}
}
