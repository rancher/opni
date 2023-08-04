package agent

import (
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/logging/apis/node"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
)

func (p *Plugin) StreamServers() []streamext.Server {
	return []streamext.Server{
		{
			Desc: &capabilityv1.Node_ServiceDesc,
			Impl: p.node,
		},
		{
			Desc: &collogspb.LogsService_ServiceDesc,
			Impl: p.otelForwarder.LogsForwarder,
		},
		{
			Desc: &coltracepb.TraceService_ServiceDesc,
			Impl: p.otelForwarder.TraceForwarder,
		},
	}
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	nodeClient := node.NewNodeLoggingCapabilityClient(cc)
	p.node.SetClient(nodeClient)
	p.otelForwarder.TraceForwarder.SetClient(cc)
	p.otelForwarder.LogsForwarder.SetClient(cc)
}
