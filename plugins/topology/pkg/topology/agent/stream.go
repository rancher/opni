package agent

import (
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	// "github.com/rancher/opni/pkg/clients"

	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/topology/pkg/apis/node"
	"github.com/rancher/opni/plugins/topology/pkg/apis/remote"

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
	p.topologyStreamer.SetRemoteWriteClient(remote.NewRemoteTopologyClient(cc))

	nodeClient := node.NewNodeTopologyCapabilityClient(cc)
	p.node.SetClient(nodeClient)
}
