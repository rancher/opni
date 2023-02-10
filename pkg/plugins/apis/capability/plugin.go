package capability

import (
	context "context"

	"github.com/hashicorp/go-plugin"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	"github.com/rancher/opni/pkg/plugins"
	"google.golang.org/grpc"
)

const (
	CapabilityBackendPluginID = "opni.backends.Capability"
	CapabilityNodePluginID    = "opni.backends.CapabilityNode"
	ServiceID                 = "capability.Backend"
	NodeServiceID             = "capability.Node"
)

type capabilityBackendPlugin struct {
	plugin.NetRPCUnsupportedPlugin

	backendSrv capabilityv1.BackendServer
}

var _ plugin.GRPCPlugin = (*capabilityBackendPlugin)(nil)
var _ plugin.Plugin = (*capabilityBackendPlugin)(nil)

func (p *capabilityBackendPlugin) GRPCServer(
	_ *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	capabilityv1.RegisterBackendServer(s, p.backendSrv)
	return nil
}

func (p *capabilityBackendPlugin) GRPCClient(
	ctx context.Context,
	_ *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	if err := plugins.CheckAvailability(ctx, c, ServiceID); err != nil {
		return nil, err
	}
	return capabilityv1.NewBackendClient(c), nil
}

func NewPlugin(backend capabilityv1.BackendServer) plugin.Plugin {
	return &capabilityBackendPlugin{
		backendSrv: backend,
	}
}

type capabilityAgentPlugin struct {
	plugin.NetRPCUnsupportedPlugin

	nodeSrv capabilityv1.NodeServer
}

var _ plugin.GRPCPlugin = (*capabilityAgentPlugin)(nil)
var _ plugin.Plugin = (*capabilityAgentPlugin)(nil)

func NewAgentPlugin(node capabilityv1.NodeServer) plugin.Plugin {
	return &capabilityAgentPlugin{
		nodeSrv: node,
	}
}

func (p *capabilityAgentPlugin) GRPCServer(
	_ *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	capabilityv1.RegisterNodeServer(s, p.nodeSrv)
	return nil
}

func (p *capabilityAgentPlugin) GRPCClient(
	ctx context.Context,
	_ *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	if err := plugins.CheckAvailability(ctx, c, NodeServiceID); err != nil {
		return nil, err
	}
	return capabilityv1.NewNodeClient(c), nil
}

func init() {
	plugins.GatewayScheme.Add(CapabilityBackendPluginID, NewPlugin(nil))
	plugins.AgentScheme.Add(CapabilityNodePluginID, NewAgentPlugin(nil))
}
