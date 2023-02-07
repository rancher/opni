package managementext

import (
	"context"

	"github.com/hashicorp/go-plugin"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/kralicky/ragu/compat"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/grpc"
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
	_ *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	apiextensions.RegisterManagementAPIExtensionServer(s, p.extensionSrv)
	for _, sp := range p.extensionSrv.services {
		s.RegisterService(sp.Unpack())
	}
	return nil
}

func (p *managementApiExtensionPlugin) GRPCClient(
	ctx context.Context,
	_ *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	if err := plugins.CheckAvailability(ctx, c, ServiceID); err != nil {
		return nil, err
	}
	return apiextensions.NewManagementAPIExtensionClient(c), nil
}

func NewPlugin(services ...util.ServicePackInterface) plugin.Plugin {
	return &managementApiExtensionPlugin{
		extensionSrv: &mgmtExtensionServerImpl{
			services: services,
		},
	}
}

type mgmtExtensionServerImpl struct {
	apiextensions.UnimplementedManagementAPIExtensionServer
	services []util.ServicePackInterface
}

func (e *mgmtExtensionServerImpl) Descriptors(_ context.Context, _ *emptypb.Empty) (*apiextensions.ServiceDescriptorProtoList, error) {
	list := &apiextensions.ServiceDescriptorProtoList{}
	for _, s := range e.services {
		rawDesc, _ := s.Unpack()
		desc, err := grpcreflect.LoadServiceDescriptor(rawDesc)
		fqn := desc.GetFullyQualifiedName()
		sd := util.ProtoClone(desc.AsServiceDescriptorProto())
		sd.Name = &fqn
		if err != nil {
			return nil, err
		}
		list.Items = append(list.Items, sd)
	}
	return list, nil
}

var _ apiextensions.ManagementAPIExtensionServer = (*mgmtExtensionServerImpl)(nil)

func init() {
	compat.LoadGogoFileDescriptor("k8s.io/api/core/v1/generated.proto")
	compat.LoadGogoFileDescriptor("k8s.io/apimachinery/pkg/api/resource/generated.proto")
	plugins.GatewayScheme.Add(ManagementAPIExtensionPluginID, NewPlugin())
}
