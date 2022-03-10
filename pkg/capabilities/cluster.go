package capabilities

import "github.com/rancher/opni-monitoring/pkg/core"

func Cluster(name string) *core.ClusterCapability {
	return &core.ClusterCapability{
		Name: name,
	}
}
