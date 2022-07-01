package gatewayunaryext

import (
	"context"

	"github.com/hashicorp/go-plugin"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	UnaryAPIExtensionPluginID = "opni.apiextensions.UnaryAPIExtension"
	ServiceID                 = "apiextensions.UnaryAPIExtension"
)

type unaryApiExtensionPlugin struct {
	plugin.NetRPCUnsupportedPlugin

	extensionSrv *unaryExtensionServerImpl
}

var _ plugin.GRPCPlugin = (*unaryApiExtensionPlugin)(nil)

func (p *unaryApiExtensionPlugin) GRPCServer(
	broker *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	apiextensions.RegisterUnaryAPIExtensionServer(s, p.extensionSrv)
	s.RegisterService(p.extensionSrv.rawServiceDesc, p.extensionSrv.serviceImpl)
	return nil
}

func (p *unaryApiExtensionPlugin) GRPCClient(
	ctx context.Context,
	broker *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	if err := plugins.CheckAvailability(ctx, c, ServiceID); err != nil {
		return nil, err
	}
	return apiextensions.NewUnaryAPIExtensionClient(c), nil
}

func NewPlugin(desc *grpc.ServiceDesc, impl interface{}) plugin.Plugin {
	descriptor, err := grpcreflect.LoadServiceDescriptor(desc)
	if err != nil {
		panic(err)
	}

	return &unaryApiExtensionPlugin{
		extensionSrv: &unaryExtensionServerImpl{
			rawServiceDesc: desc,
			serviceDesc:    descriptor,
			serviceImpl:    impl,
		},
	}
}

type unaryExtensionServerImpl struct {
	apiextensions.UnimplementedUnaryAPIExtensionServer
	rawServiceDesc *grpc.ServiceDesc
	serviceDesc    *desc.ServiceDescriptor
	serviceImpl    interface{}
}

func (e *unaryExtensionServerImpl) UnaryDescriptor(ctx context.Context, _ *emptypb.Empty) (*descriptorpb.ServiceDescriptorProto, error) {
	fqn := e.serviceDesc.GetFullyQualifiedName()
	sd := e.serviceDesc.AsServiceDescriptorProto()
	sd.Name = &fqn
	return sd, nil
}

var _ apiextensions.UnaryAPIExtensionServer = (*unaryExtensionServerImpl)(nil)

func init() {
	plugins.ClientScheme.Add(UnaryAPIExtensionPluginID,
		NewPlugin(&apiextensions.UnaryAPIExtension_ServiceDesc,
			apiextensions.UnimplementedUnaryAPIExtensionServer{}))
}
