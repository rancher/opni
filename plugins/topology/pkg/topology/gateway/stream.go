package gateway

import (
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/topology/pkg/apis/node"
	"github.com/rancher/opni/plugins/topology/pkg/apis/stream"
)

func (p *Plugin) StreamServers() []streamext.Server {
	return []streamext.Server{
		{
			Desc:              &stream.RemoteTopology_ServiceDesc,
			Impl:              &p.topologyRemoteWrite,
			RequireCapability: wellknown.CapabilityTopology,
		},
		{
			Desc:              &node.NodeTopologyCapability_ServiceDesc,
			Impl:              &p.topologyBackend,
			RequireCapability: wellknown.CapabilityTopology,
		},
	}
}
