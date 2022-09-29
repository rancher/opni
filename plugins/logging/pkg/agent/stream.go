package agent

import (
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
)

func (p *Plugin) StreamServers() []streamext.Server {
	return []streamext.Server{
		{
			Desc: &capabilityv1.Node_ServiceDesc,
			Impl: p.node,
		},
	}
}
