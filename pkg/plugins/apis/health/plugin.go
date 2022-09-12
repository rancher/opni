package health

import (
	"context"

	"github.com/hashicorp/go-plugin"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/plugins"
	"google.golang.org/grpc"
)

const (
	HealthPluginID = "opni.Health"
	ServiceID      = "control.Health"
)

type healthPlugin struct {
	plugin.NetRPCUnsupportedPlugin

	srv controlv1.HealthServer
}

func NewPlugin(srv controlv1.HealthServer) plugin.Plugin {
	return &healthPlugin{
		srv: srv,
	}
}

func (p *healthPlugin) GRPCServer(
	broker *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	controlv1.RegisterHealthServer(s, p.srv)
	return nil
}

func (p *healthPlugin) GRPCClient(
	ctx context.Context,
	broker *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	if err := plugins.CheckAvailability(ctx, c, ServiceID); err != nil {
		return nil, err
	}
	return controlv1.NewHealthClient(c), nil
}

func init() {
	plugins.AgentScheme.Add(HealthPluginID, NewPlugin(nil))
}
