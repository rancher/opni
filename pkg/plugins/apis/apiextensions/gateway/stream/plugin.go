package gatewayext

import (
	"context"

	"github.com/hashicorp/go-plugin"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/kralicky/totem"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

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

type Server struct {
	Desc              *grpc.ServiceDesc
	Impl              interface{}
	RequireCapability string
}

type richServer struct {
	Server
	richDesc *desc.ServiceDescriptor
}

type streamApiExtensionPlugin struct {
	plugin.NetRPCUnsupportedPlugin

	extensionSrv *mgmtExtensionServerImpl
}

var _ plugin.GRPCPlugin = (*streamApiExtensionPlugin)(nil)

func (p *streamApiExtensionPlugin) GRPCServer(
	broker *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	apiextensions.RegisterStreamAPIExtensionServer(s, p.extensionSrv)
	streamv1.RegisterStreamServer(s, p.extensionSrv)
	return nil
}

func (p *streamApiExtensionPlugin) GRPCClient(
	ctx context.Context,
	broker *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	if err := plugins.CheckAvailability(ctx, c, ServiceID); err != nil {
		return nil, err
	}
	return apiextensions.NewStreamAPIExtensionClient(c), nil
}

func NewPlugin(p StreamAPIExtension) plugin.Plugin {
	ext := &mgmtExtensionServerImpl{}
	if p != nil {
		servers := p.StreamServers()
		for _, srv := range servers {
			descriptor, err := grpcreflect.LoadServiceDescriptor(srv.Desc)
			if err != nil {
				panic(err)
			}
			ext.servers = append(ext.servers, &richServer{
				Server:   srv,
				richDesc: descriptor,
			})
		}
	}
	return &streamApiExtensionPlugin{
		extensionSrv: ext,
	}
}

type mgmtExtensionServerImpl struct {
	streamv1.UnsafeStreamServer
	apiextensions.UnimplementedStreamAPIExtensionServer
	servers []*richServer
}

func (e *mgmtExtensionServerImpl) Connect(stream streamv1.Stream_ConnectServer) error {
	ts := totem.NewServer(stream)
	for _, srv := range e.servers {
		ts.RegisterService(srv.Desc, srv.Impl)
	}
	_, errC := ts.Serve()
	return <-errC
}

func (e *mgmtExtensionServerImpl) Services(context.Context, *emptypb.Empty) (*apiextensions.ServiceDescriptorList, error) {
	list := []*apiextensions.ServiceDescriptor{}
	for _, srv := range e.servers {
		fqn := srv.richDesc.GetFullyQualifiedName()
		sd := srv.richDesc.AsServiceDescriptorProto()
		sd.Name = &fqn
		list = append(list, &apiextensions.ServiceDescriptor{
			ServiceDescriptor: sd,
			Options: &apiextensions.ServiceOptions{
				RequireCapability: srv.RequireCapability,
			},
		})
	}
	return &apiextensions.ServiceDescriptorList{
		Items: list,
	}, nil
}

var _ apiextensions.StreamAPIExtensionServer = (*mgmtExtensionServerImpl)(nil)

func init() {
	plugins.ClientScheme.Add(StreamAPIExtensionPluginID, NewPlugin(nil))
}
