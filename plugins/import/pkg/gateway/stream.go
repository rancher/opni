package gateway

import (
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/import/pkg/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	"google.golang.org/grpc"
)

func (p *Plugin) StreamServers() []streamext.Server {
	return []streamext.Server{

		{
			Desc:              &remotewrite.RemoteWrite_ServiceDesc,
			Impl:              &p.cortexRemoteWrite,
			RequireCapability: wellknown.CapabilityMetrics,
		},
		{
			Desc:              &remoteread.RemoteReadGateway_ServiceDesc,
			Impl:              &p.importBackend,
			RequireCapability: wellknown.CapabilityMetrics,
		},
	}
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	p.delegate.Set(streamext.NewDelegate(cc, remoteread.NewRemoteReadAgentClient))
}
