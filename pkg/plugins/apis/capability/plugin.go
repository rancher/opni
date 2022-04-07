package capability

import (
	context "context"

	"github.com/hashicorp/go-plugin"
	core "github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type Backend interface {
	// Returns an error if installing the capability would fail.
	CanInstall() error
	// Any error returned from this method is fatal.
	Install(cluster *core.Reference) error
	// Returns a go template string which will generate a shell command used to
	// install the capability. This will be displayed to the user in the UI.
	// See InstallerTemplateSpec above for the available template fields.
	InstallerTemplate() string
}

const (
	CapabilityBackendPluginID = "opni.backends.Capability"
	ServiceID                 = "capability.Backend"
)

type capabilityBackendPlugin struct {
	plugin.NetRPCUnsupportedPlugin

	backendSrv *backendServerImpl
}

var _ plugin.GRPCPlugin = (*capabilityBackendPlugin)(nil)
var _ plugin.Plugin = (*capabilityBackendPlugin)(nil)

func (p *capabilityBackendPlugin) GRPCServer(
	broker *plugin.GRPCBroker,
	s *grpc.Server,
) error {
	RegisterBackendServer(s, p.backendSrv)
	return nil
}

func (p *capabilityBackendPlugin) GRPCClient(
	ctx context.Context,
	broker *plugin.GRPCBroker,
	c *grpc.ClientConn,
) (interface{}, error) {
	if err := plugins.CheckAvailability(ctx, c, ServiceID); err != nil {
		return nil, err
	}
	return NewBackendClient(c), nil
}

type backendServerImpl struct {
	UnsafeBackendServer

	capabilityName string
	impl           Backend
}

func (b *backendServerImpl) Info(
	ctx context.Context,
	in *emptypb.Empty,
) (*InfoResponse, error) {
	return &InfoResponse{
		CapabilityName: b.capabilityName,
	}, nil
}

func (b *backendServerImpl) CanInstall(
	ctx context.Context,
	in *emptypb.Empty,
) (*emptypb.Empty, error) {
	err := b.impl.CanInstall()
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (b *backendServerImpl) Install(
	ctx context.Context,
	in *InstallRequest,
) (*emptypb.Empty, error) {
	err := b.impl.Install(in.Cluster)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (b *backendServerImpl) Uninstall(
	ctx context.Context,
	in *emptypb.Empty,
) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Uninstall not implemented")
}

func (b *backendServerImpl) InstallerTemplate(
	ctx context.Context,
	in *emptypb.Empty,
) (*InstallerTemplateResponse, error) {
	return &InstallerTemplateResponse{
		Template: b.impl.InstallerTemplate(),
	}, nil
}

func NewPlugin(capabilityName string, backend Backend) plugin.Plugin {
	return &capabilityBackendPlugin{
		backendSrv: &backendServerImpl{
			capabilityName: capabilityName,
			impl:           backend,
		},
	}
}

func init() {
	plugins.ClientScheme.Add(CapabilityBackendPluginID, NewPlugin("", nil))
}
