package alerting

import (
	"github.com/rancher/opni/pkg/agent"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/rules"
	"google.golang.org/grpc"
)

func (p *Plugin) StreamServers() []util.ServicePackInterface {
	return []util.ServicePackInterface{
		util.PackService[node.NodeAlertingCapabilityServer](&node.NodeAlertingCapability_ServiceDesc, &p.node),
		util.PackService[rules.RuleSyncServer](&rules.RuleSync_ServiceDesc, p.AlarmServerComponent),
	}
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	p.delegate.Set(streamext.NewDelegate(cc, agent.NewClientSet))
}
