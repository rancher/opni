package backendhealth

import (
	"context"

	"github.com/hashicorp/go-plugin"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/plugins"
	"google.golang.org/grpc"
)

const (
	BackendHealthPluginID = "opni.BackendHealth"
	ServiceID             = "control.BackendHealth"
)

type healthPlugin struct {
	plugin.NetRPCUnsupportedPlugin

	srv controlv1.BackendHealthServer
}

func NewPlugin(srv controlv1.BackendHealthServer) plugin.Plugin {
	return &healthPlugin{
		srv: srv,
	}
}

func (p *healthPlugin) GRPCServer(
	broker *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	controlv1.RegisterBackendHealthServer(s, p.srv)
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
	return controlv1.NewBackendHealthClient(c), nil
}

func init() {
	plugins.AgentScheme.Add(BackendHealthPluginID, NewPlugin(nil))
}
