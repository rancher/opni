package agent

import (
	"github.com/rancher/opni/pkg/clients"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	"google.golang.org/grpc"
)

func (p *Plugin) StreamServers() []streamext.Server {
	return []streamext.Server{}
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	p.httpServer.SetRemoteWriteClient(clients.NewLocker(cc, remotewrite.NewRemoteWriteClient))
}
