package cortex

import (
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
)

func (p *Plugin) StreamServers() []streamext.Server {
	return []streamext.Server{
		{
			Desc: &remotewrite.RemoteWrite_ServiceDesc,
			Impl: &remoteWriteForwarder{
				client: p.cortexClientSet,
				config: p.config,
				logger: p.logger,
			},
			RequireCapability: wellknown.CapabilityMetrics,
		},
	}
}
