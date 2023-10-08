package agent

import (
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/logging/apis/node"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/grpc"
)

func (p *Plugin) StreamServers() []util.ServicePackInterface {
	return []util.ServicePackInterface{
		util.PackService[capabilityv1.NodeServer](&capabilityv1.Node_ServiceDesc, p.node),
		util.PackService[collogspb.LogsServiceServer](&collogspb.LogsService_ServiceDesc, p.otelForwarder),
	}
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	nodeClient := node.NewNodeLoggingCapabilityClient(cc)
	p.node.SetClient(nodeClient)
	p.otelForwarder.SetClient(cc)
}
