package alerting

import (
	"github.com/rancher/opni/pkg/agent"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/rules"
	"google.golang.org/grpc"
)

func (p *Plugin) StreamServers() []streamext.Server {
	return []streamext.Server{
		{
			Desc:              &node.NodeAlertingCapability_ServiceDesc,
			Impl:              &p.node,
			RequireCapability: wellknown.CapabilityAlerting,
		},
		{
			Desc:              &rules.RuleSync_ServiceDesc,
			Impl:              p.AlarmServerComponent,
			RequireCapability: wellknown.CapabilityAlerting,
		},
	}
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	p.delegate.Set(streamext.NewDelegate(cc, agent.NewClientSet))
}
