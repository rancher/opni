package agent

import (
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/logger"
	streamext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/stream"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/rules"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) StreamServers() []streamext.Server {
	return []streamext.Server{
		{
			Desc: &capabilityv1.Node_ServiceDesc,
			Impl: p.node,
		},
	}
}

func (p *Plugin) UseStreamClient(cc grpc.ClientConnInterface) {
	nodeClient := node.NewNodeAlertingCapabilityClient(cc)
	healthListenerClient := controlv1.NewHealthListenerClient(cc)
	identityClient := controlv1.NewIdentityClient(cc)
	ruleSyncClient := rules.NewRuleSyncClient(cc)

	p.configureLoggers(identityClient)

	p.node.SetClients(
		healthListenerClient,
		nodeClient,
		identityClient,
	)

	p.ruleStreamer.SetClients(ruleSyncClient)
}

func (p *Plugin) configureLoggers(identityClient controlv1.IdentityClient) {
	id, err := identityClient.Whoami(p.ctx, &emptypb.Empty{})
	if err != nil {
		p.lg.Error("error fetching node id", logger.Err(err))
	}
	logger.InitPluginWriter(id.GetId())
}
