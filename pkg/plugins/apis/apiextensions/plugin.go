package apiextensions

import (
	context "context"

	"github.com/hashicorp/go-plugin"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"google.golang.org/grpc"
	descriptorpb "google.golang.org/protobuf/types/descriptorpb"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type managementApiExtensionPlugin struct {
	plugin.NetRPCUnsupportedPlugin

	extensionSrv *apiExtensionServerImpl
}

func (p *managementApiExtensionPlugin) GRPCServer(
	broker *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	RegisterManagementAPIExtensionServer(s, p.extensionSrv)
	s.RegisterService(p.extensionSrv.rawServiceDesc, p.extensionSrv.serviceImpl)
	return nil
}

func (p *managementApiExtensionPlugin) GRPCClient(
	ctx context.Context,
	broker *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	return NewManagementAPIExtensionClient(c), nil
}

func NewPlugin(desc *grpc.ServiceDesc, impl interface{}) plugin.Plugin {
	descriptor, err := grpcreflect.LoadServiceDescriptor(desc)
	if err != nil {
		panic(err)
	}

	return &managementApiExtensionPlugin{
		extensionSrv: &apiExtensionServerImpl{
			rawServiceDesc: desc,
			serviceDesc:    descriptor,
			serviceImpl:    impl,
		},
	}
}

const ManagementAPIExtensionPluginID = "apiextensions.ManagementAPIExtension"

type apiExtensionServerImpl struct {
	UnimplementedManagementAPIExtensionServer
	rawServiceDesc *grpc.ServiceDesc
	serviceDesc    *desc.ServiceDescriptor
	serviceImpl    interface{}
}

func (e *apiExtensionServerImpl) Descriptor(ctx context.Context, _ *emptypb.Empty) (*descriptorpb.ServiceDescriptorProto, error) {
	fqn := e.serviceDesc.GetFullyQualifiedName()
	sd := e.serviceDesc.AsServiceDescriptorProto()
	sd.Name = &fqn
	return sd, nil
}

var _ ManagementAPIExtensionServer = (*apiExtensionServerImpl)(nil)

func init() {
	plugins.Scheme.Add(ManagementAPIExtensionPluginID,
		NewPlugin(&ManagementAPIExtension_ServiceDesc, UnimplementedManagementAPIExtensionServer{}))
}
