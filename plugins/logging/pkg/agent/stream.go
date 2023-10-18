package agent

import (
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/logger"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/logging/apis/node"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) StreamServers() []streamext.Server {
	return []streamext.Server{
		{
			Desc:              &capabilityv1.Node_ServiceDesc,
			Impl:              p.node,
			RequireCapability: wellknown.CapabilityLogs,
		},
		{
			Desc:              &collogspb.LogsService_ServiceDesc,
			Impl:              p.otelForwarder.LogsForwarder,
			RequireCapability: wellknown.CapabilityLogs,
		},
		{
			Desc:              &coltracepb.TraceService_ServiceDesc,
			Impl:              p.otelForwarder.TraceForwarder,
			RequireCapability: wellknown.CapabilityLogs,
		},
	}
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	p.configureLoggers(cc)
	nodeClient := node.NewNodeLoggingCapabilityClient(cc)
	p.node.SetClient(nodeClient)
	p.otelForwarder.TraceForwarder.SetClient(cc)
	p.otelForwarder.LogsForwarder.SetClient(cc)
}

func (p *Plugin) configureLoggers(cc grpc.ClientConnInterface) {
	identityClient := controlv1.NewIdentityClient(cc)
	id, err := identityClient.Whoami(p.ctx, &emptypb.Empty{})
	if err != nil {
		p.logger.Error("error fetching node id", logger.Err(err))
	}
	logger.InitPluginWriter(id.GetId())
}
