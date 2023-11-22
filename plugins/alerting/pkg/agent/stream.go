package agent

import (
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/rules"
	"google.golang.org/grpc"
)

func (p *Plugin) StreamServers() []util.ServicePackInterface {
	return []util.ServicePackInterface{
		util.PackService[capabilityv1.NodeServer](&capabilityv1.Node_ServiceDesc, p.node),
	}
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	nodeClient := node.NewNodeAlertingCapabilityClient(cc)
	healthListenerClient := controlv1.NewHealthListenerClient(cc)
	identityClient := controlv1.NewIdentityClient(cc)
	ruleSyncClient := rules.NewRuleSyncClient(cc)

	p.node.SetClients(
		healthListenerClient,
		nodeClient,
		identityClient,
	)

	p.ruleStreamer.SetClients(ruleSyncClient)
}
