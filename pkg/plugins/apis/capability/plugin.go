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
	ServiceID                 = "capability.Backend"
)

type capabilityBackendPlugin struct {
	plugin.NetRPCUnsupportedPlugin

	backendSrv capabilityv1.BackendServer
}

var _ plugin.GRPCPlugin = (*capabilityBackendPlugin)(nil)
var _ plugin.Plugin = (*capabilityBackendPlugin)(nil)

func (p *capabilityBackendPlugin) GRPCServer(
	broker *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	capabilityv1.RegisterBackendServer(s, p.backendSrv)
	return nil
}

func (p *capabilityBackendPlugin) GRPCClient(
	ctx context.Context,
	broker *plugin.GRPCBroker,
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

func init() {
	plugins.ClientScheme.Add(CapabilityBackendPluginID, NewPlugin(nil))
}
