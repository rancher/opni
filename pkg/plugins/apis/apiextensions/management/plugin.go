package managementext

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
	ManagementAPIExtensionPluginID = "opni.apiextensions.ManagementAPIExtension"
	ServiceID                      = "apiextensions.ManagementAPIExtension"
)

type managementApiExtensionPlugin struct {
	plugin.NetRPCUnsupportedPlugin

	extensionSrv *mgmtExtensionServerImpl
}

var _ plugin.GRPCPlugin = (*managementApiExtensionPlugin)(nil)

func (p *managementApiExtensionPlugin) GRPCServer(
	broker *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	apiextensions.RegisterManagementAPIExtensionServer(s, p.extensionSrv)
	s.RegisterService(p.extensionSrv.rawServiceDesc, p.extensionSrv.serviceImpl)
	return nil
}

func (p *managementApiExtensionPlugin) GRPCClient(
	ctx context.Context,
	broker *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	if err := plugins.CheckAvailability(ctx, c, ServiceID); err != nil {
		return nil, err
	}
	return apiextensions.NewManagementAPIExtensionClient(c), nil
}

func NewPlugin(desc *grpc.ServiceDesc, impl interface{}) plugin.Plugin {
	descriptor, err := grpcreflect.LoadServiceDescriptor(desc)
	if err != nil {
		panic(err)
	}

	return &managementApiExtensionPlugin{
		extensionSrv: &mgmtExtensionServerImpl{
			rawServiceDesc: desc,
			serviceDesc:    descriptor,
			serviceImpl:    impl,
		},
	}
}

type mgmtExtensionServerImpl struct {
	apiextensions.UnimplementedManagementAPIExtensionServer
	rawServiceDesc *grpc.ServiceDesc
	serviceDesc    *desc.ServiceDescriptor
	serviceImpl    interface{}
}

func (e *mgmtExtensionServerImpl) Descriptor(ctx context.Context, _ *emptypb.Empty) (*descriptorpb.ServiceDescriptorProto, error) {
	fqn := e.serviceDesc.GetFullyQualifiedName()
	sd := e.serviceDesc.AsServiceDescriptorProto()
	sd.Name = &fqn
	return sd, nil
}

var _ apiextensions.ManagementAPIExtensionServer = (*mgmtExtensionServerImpl)(nil)

func init() {
	plugins.ClientScheme.Add(ManagementAPIExtensionPluginID,
		NewPlugin(&apiextensions.ManagementAPIExtension_ServiceDesc,
			apiextensions.UnimplementedManagementAPIExtensionServer{}))
}
