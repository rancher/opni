package cortex

import (
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/gateway/stream"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/remotewrite"
)

func (p *Plugin) StreamServers() []streamext.Server {
	return []streamext.Server{
		{
			Desc: &remotewrite.RemoteWrite_ServiceDesc,
			Impl: &remoteWriteForwarder{
				distClient: p.distributorClient,
				httpClient: p.cortexHttpClient,
				config:     p.config,
				logger:     p.logger,
			},
			RequireCapability: wellknown.CapabilityMetrics,
		},
	}
}
