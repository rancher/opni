package stream

import (
	"context"

	"github.com/hashicorp/go-plugin"
	"github.com/jhump/protoreflect/desc"
	"google.golang.org/grpc"

	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
)

const (
	StreamAPIExtensionPluginID = "opni.apiextensions.StreamAPIExtension"
	ServiceID                  = "apiextensions.StreamAPIExtension"
)

type StreamAPIExtension interface {
	StreamServers() []Server
}

// A plugin can optionally implement StreamClientHandler to obtain a
// grpc.ClientConnInterface to the plugin's side of the spliced stream.
type StreamClientHandler interface {
	UseStreamClient(client grpc.ClientConnInterface)
}

type StreamAPIExtensionWithHandlers interface {
	StreamAPIExtension
	StreamClientHandler
}

type Server struct {
	Desc              *grpc.ServiceDesc
	Impl              interface{}
	RequireCapability string
}

type richServer struct {
	Server
	richDesc *desc.ServiceDescriptor
}

type streamExtPlugin interface {
	streamv1.StreamServer
	apiextensions.StreamAPIExtensionServer
}

type streamApiExtensionPlugin[T streamExtPlugin] struct {
	plugin.NetRPCUnsupportedPlugin
	GatewayStreamApiExtensionPluginOptions

	extensionSrv T
}

func (p *streamApiExtensionPlugin[T]) GRPCServer(
	_ *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	apiextensions.RegisterStreamAPIExtensionServer(s, p.extensionSrv)
	streamv1.RegisterStreamServer(s, p.extensionSrv)
	return nil
}

func (p *streamApiExtensionPlugin[T]) GRPCClient(
	ctx context.Context,
	_ *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	if err := plugins.CheckAvailability(ctx, c, ServiceID); err != nil {
		return nil, err
	}
	return apiextensions.NewStreamAPIExtensionClient(c), nil
}

func init() {
	plugins.GatewayScheme.Add(StreamAPIExtensionPluginID, NewGatewayPlugin(nil))
	plugins.AgentScheme.Add(StreamAPIExtensionPluginID, NewAgentPlugin(nil))
}
