package gateway

import (
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/logging/pkg/apis/node"
)

func (p *Plugin) StreamServers() []streamext.Server {
	return []streamext.Server{
		{
			Desc:              &node.NodeLoggingCapability_ServiceDesc,
			Impl:              &p.logging,
			RequireCapability: wellknown.CapabilityMetrics,
		},
	}
}
