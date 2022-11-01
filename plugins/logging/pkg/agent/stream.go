package agent

import (
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/logging/pkg/apis/node"
	"google.golang.org/grpc"
)

func (p *Plugin) StreamServers() []streamext.Server {
	return []streamext.Server{
		{
			Desc: &capabilityv1.Node_ServiceDesc,
			Impl: p.node,
		},
	}
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	nodeClient := node.NewNodeLoggingCapabilityClient(cc)
	p.node.SetClient(nodeClient)
}

func (p *Plugin) StreamDisconnected() {
	config := &node.LoggingCapabilityConfig{
		Enabled: false,
	}
	p.node.updateConfig(config)
}
