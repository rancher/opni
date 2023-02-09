package gateway

import (
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
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
			Impl:              &p.metrics,
			RequireCapability: wellknown.CapabilityMetrics,
		},
		{
			Desc:              &node.NodeMetricsCapability_ServiceDesc,
			Impl:              &p.metrics,
			RequireCapability: wellknown.CapabilityMetrics,
		},
	}
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	// example delegate usage
	// syncDelegate := streamext.NewDelegate(cc, capabilityv1.NewNodeClient)
	// target := &corev1.Reference{Id: "foo"}
	// syncDelegate.WithTarget(target).SyncNow(context.Background(), &capabilityv1.Filter{})
}
