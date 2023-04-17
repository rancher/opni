package agent

import (
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/clients"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/grpc"
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
		{
			Desc: &colmetricspb.MetricsService_ServiceDesc,
			Impl: &p.otelForwarder,
		},
	}
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	nodeClient := node.NewNodeMetricsCapabilityClient(cc)
	healthListenerClient := controlv1.NewHealthListenerClient(cc)
	identityClient := controlv1.NewIdentityClient(cc)
	colMetricsClient := colmetricspb.NewMetricsServiceClient(cc)

	p.httpServer.SetRemoteWriteClient(clients.NewLocker(cc, remotewrite.NewRemoteWriteClient))
	p.ruleStreamer.SetRemoteWriteClient(remotewrite.NewRemoteWriteClient(cc))
	p.node.SetRemoteWriter(clients.NewLocker(cc, remotewrite.NewRemoteWriteClient))
	p.otelForwarder.SetClient(colMetricsClient)
	p.node.SetClients(
		nodeClient,
		identityClient,
		healthListenerClient,
	)
}
