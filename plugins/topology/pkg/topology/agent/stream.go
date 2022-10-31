package agent

import (
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	// "github.com/rancher/opni/pkg/clients"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"

	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/topology/pkg/apis/node"
	"github.com/rancher/opni/plugins/topology/pkg/apis/stream"

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
	p.topologyStreamer.SetTopologyStreamClient(stream.NewRemoteTopologyClient(cc))
	p.topologyStreamer.SetIdentityClient(controlv1.NewIdentityClient(cc))
	p.node.SetClient(node.NewNodeTopologyCapabilityClient(cc))
}
