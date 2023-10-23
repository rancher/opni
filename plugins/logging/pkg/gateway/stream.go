package gateway

import (
	"github.com/rancher/opni/pkg/agent"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/logging/apis/node"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/grpc"
)

func (p *Plugin) StreamServers() []util.ServicePackInterface {
	return []util.ServicePackInterface{
		util.PackService[node.NodeLoggingCapabilityServer](&node.NodeLoggingCapability_ServiceDesc, &p.logging),
		util.PackService[collogspb.LogsServiceServer](&collogspb.LogsService_ServiceDesc, p.otelForwarder),
	}
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	p.delegate.Set(streamext.NewDelegate(cc, agent.NewClientSet))
}
