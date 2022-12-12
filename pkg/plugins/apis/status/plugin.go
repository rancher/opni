package status

import (
	"context"

	"github.com/hashicorp/go-plugin"
	statusv1 "github.com/rancher/opni/pkg/apis/status/v1"
	"github.com/rancher/opni/pkg/plugins"
	"google.golang.org/grpc"
)

const (
	StatusPluginID = "opni.Status"
	ServiceID      = "status.Status"
)

type statusPlugin struct {
	plugin.NetRPCUnsupportedPlugin

	srv statusv1.StatusServer
}

func NewPlugin(srv statusv1.StatusServer) plugin.Plugin {
	return &statusPlugin{
		srv: srv,
	}
}

func (p *statusPlugin) GRPCServer(
	broker *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	statusv1.RegisterStatusServer(s, p.srv)
	return nil
}

func (p *statusPlugin) GRPCClient(
	ctx context.Context,
	broker *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	if err := plugins.CheckAvailability(ctx, c, ServiceID); err != nil {
		return nil, err
	}
	return statusv1.NewStatusClient(c), nil
}

func init() {
	plugins.AgentScheme.Add(StatusPluginID, NewPlugin(nil))
}
