package agent

import (
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/clients"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	"google.golang.org/grpc"
)

func (p *Plugin) StreamServers() []streamext.Server {
	return []streamext.Server{
		{
			Desc: &capabilityv1.Node_ServiceDesc,
			Impl: p.node,
		},
		{
			Desc: &remoteread.RemoteReadGateway_ServiceDesc,
			Impl: p.node,
		},
	}
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	p.httpServer.SetRemoteWriteClient(clients.NewLocker(cc, remotewrite.NewRemoteWriteClient))
	p.httpServer.SetTargetRunner(clients.NewLocker(cc, func(_ grpc.ClientConnInterface) TargetRunner {
		return NewTargetRunner(p.logger.Named("remote-read"))
	}))
	p.httpServer.SetRemoteReadClient(clients.NewLocker(cc, remoteread.NewRemoteReadGatewayClient))

	p.ruleStreamer.SetRemoteWriteClient(remotewrite.NewRemoteWriteClient(cc))

	nodeClient := node.NewNodeMetricsCapabilityClient(cc)
	healthListenerClient := controlv1.NewHealthListenerClient(cc)
	identityClient := controlv1.NewIdentityClient(cc)
	remoteReadClient := remoteread.NewRemoteReadGatewayClient(cc)

	p.node.SetNodeClient(nodeClient)
	p.node.SetHealthListenerClient(healthListenerClient)
	p.node.SetIdentityClient(identityClient)
	p.node.SetRemoteReadClient(remoteReadClient)
}
