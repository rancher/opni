package managementext

import (
	"context"

	"github.com/hashicorp/go-plugin"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	ManagementAPIExtensionPluginID = "opni.apiextensions.ManagementAPIExtension"
	ServiceID                      = "apiextensions.ManagementAPIExtension"
)

const (
	Serving        = healthpb.HealthCheckResponse_SERVING
	NotServing     = healthpb.HealthCheckResponse_NOT_SERVING
	ServiceUnknown = healthpb.HealthCheckResponse_SERVICE_UNKNOWN
)

// ServiceController is an interface to the grpc health server which monitors
// the serving status of all api extension services for this plugin.
type ServiceController interface {
	// Changes the status of the given service. If the status is set to NotServing,
	// messages will be rejected upstream. If the status is set to Serving, messages
	// will be delivered as normal.
	SetServingStatus(service string, servingStatus healthpb.HealthCheckResponse_ServingStatus)

	// Shutdown sets all serving status to NotServing, and configures the server to
	// ignore all future status changes made via SetServingStatus().
	//
	// This changes serving status for all services. To set status for a particular
	// services, call SetServingStatus().
	Shutdown()

	// Resume sets all serving status to Serving, and configures the server to
	// accept all future status changes made via SetServingStatus().
	//
	// This changes serving status for all services. To set status for a particular
	// services, call SetServingStatus().
	Resume()
}

type SingleServiceController interface {
	// Changes the status of the given service. If the status is set to NotServing,
	// messages will be rejected upstream. If the status is set to Serving, messages
	// will be delivered as normal. The status Unknown is currently equivalent
	// to Serving.
	SetServingStatus(servingStatus healthpb.HealthCheckResponse_ServingStatus)
}

func NewSingleServiceController(srv ServiceController, service string) SingleServiceController {
	return &singleServiceController{
		srv:     srv,
		service: service,
	}
}

type singleServiceController struct {
	srv     ServiceController
	service string
}

func (s *singleServiceController) SetServingStatus(status healthpb.HealthCheckResponse_ServingStatus) {
	s.srv.SetServingStatus(s.service, status)
}

type ManagementAPIExtension interface {
	// Called by the plugin system on startup. Should return a list of management
	// services served by this plugin.
	// The service controller can be used to set the serving status of individual
	// services served by this plugin. Services must be returned by this method
	// to be controllable by the service controller.
	// The default serving state is Unknown, which is equivalent to Serving.
	// Doing nothing with the service controller is valid, and will result in all
	// services being enabled by default (however, it is still recommended to set
	// them explicitly).
	ManagementServices(s ServiceController) []util.ServicePackInterface
}

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
	for _, sp := range p.extensionSrv.ManagementServices(p.extensionSrv.healthSrv) {
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

func NewPlugin(ext ManagementAPIExtension) plugin.Plugin {
	return &managementApiExtensionPlugin{
		extensionSrv: &mgmtExtensionServerImpl{
			ManagementAPIExtension: ext,
			healthSrv:              health.NewServer(),
		},
	}
}

type mgmtExtensionServerImpl struct {
	ManagementAPIExtension
	apiextensions.UnsafeManagementAPIExtensionServer
	healthSrv *health.Server
}

// CheckHealth implements apiextensions.ManagementAPIExtensionServer.
func (e *mgmtExtensionServerImpl) CheckHealth(ctx context.Context, req *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	return e.healthSrv.Check(ctx, req)
}

// WatchHealth implements apiextensions.ManagementAPIExtensionServer.
func (e *mgmtExtensionServerImpl) WatchHealth(req *healthpb.HealthCheckRequest, stream apiextensions.ManagementAPIExtension_WatchHealthServer) error {
	return e.healthSrv.Watch(req, stream)
}

func (e *mgmtExtensionServerImpl) Descriptors(_ context.Context, _ *emptypb.Empty) (*apiextensions.ServiceDescriptorProtoList, error) {
	list := &apiextensions.ServiceDescriptorProtoList{}
	for _, s := range e.ManagementServices(e.healthSrv) {
		rawDesc, _ := s.Unpack()
		desc, err := grpcreflect.LoadServiceDescriptor(rawDesc)
		if err != nil {
			return nil, err
		}
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
	plugins.GatewayScheme.Add(ManagementAPIExtensionPluginID, NewPlugin(nil))
}
