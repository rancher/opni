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
	"net/http"
)

func (p *Plugin) StreamServers() []streamext.Server {
	return []streamext.Server{
		{
			Desc: &capabilityv1.Node_ServiceDesc,
			Impl: p.node,
		},
		{
			Desc: &remoteread.RemoteReadAgent_ServiceDesc,
			Impl: p.node,
		},
	}
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	remoteWriter := clients.NewLocker(cc, remotewrite.NewRemoteWriteClient)
	remoteReadClient := clients.NewLocker(cc, remoteread.NewRemoteReadGatewayClient)

	runner := NewTargetRunner(p.logger.Named("runner"))
	runner.SetRemoteWriteClient(remoteWriter)
	runner.SetRemoteReadClient(remoteReadClient)
	runner.SetRemoteReader(clients.NewLocker(nil, func(connInterface grpc.ClientConnInterface) RemoteReader {
		return NewRemoteReader(&http.Client{})
	}))

	nodeClient := node.NewNodeMetricsCapabilityClient(cc)
	healthListenerClient := controlv1.NewHealthListenerClient(cc)
	identityClient := controlv1.NewIdentityClient(cc)

	p.httpServer.SetRemoteWriteClient(remoteWriter)
	p.httpServer.SetRemoteReadClient(remoteReadClient)
	p.ruleStreamer.SetRemoteWriteClient(remotewrite.NewRemoteWriteClient(cc))

	p.node.SetNodeClient(nodeClient)
	p.node.SetHealthListenerClient(healthListenerClient)
	p.node.SetIdentityClient(identityClient)
	p.node.SetTargetRunner(runner)
}
