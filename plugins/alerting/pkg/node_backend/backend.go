package node_backend

import (
	"context"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
)

type AlertingNodeBackend struct {
	capabilityv1.UnsafeBackendServer
	node.UnsafeNodeAlertingCapabilityServer
	node.UnsafeNodeConfigurationServer
}

func NewAlertingNodeBackend() *AlertingNodeBackend {
	return &AlertingNodeBackend{}
}

var (
	_ node.NodeAlertingCapabilityServer = (*AlertingNodeBackend)(nil)
	_ node.NodeConfigurationServer      = (*AlertingNodeBackend)(nil)
	_ capabilityv1.BackendServer        = (*AlertingNodeBackend)(nil)
)

func (a *AlertingNodeBackend) GetDefaultConfiguration(_ context.Context, _ *emptypb.Empty) (*node.AlertingCapabilitySpec, error) {
	return nil, status.Error(codes.Unimplemented, "Alerting capability not yet supported")
}

func (a *AlertingNodeBackend) SetDefaultConfiguration(_ context.Context, _ *node.AlertingCapabilitySpec) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "Alerting capability not yet supported")
}

func (a *AlertingNodeBackend) GetNodeConfiguration(_ context.Context, _ *corev1.Reference) (*node.AlertingCapabilitySpec, error) {
	return nil, status.Error(codes.Unimplemented, "Alerting capability not yet supported")
}

func (a *AlertingNodeBackend) SetNodeConfiguration(_ context.Context, _ *node.NodeConfigRequest) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "Alerting capability not yet supported")
}

func (a *AlertingNodeBackend) Sync(_ context.Context, _ *node.AlertingCapabilityConfig) (*node.SyncResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Alerting capability not yet supported")
}

// Returns info about the backend, including capability name
func (a *AlertingNodeBackend) Info(_ context.Context, _ *emptypb.Empty) (*capabilityv1.Details, error) {
	return nil, status.Error(codes.Unimplemented, "Alerting capability not yet supported")
}

// Deprecated: Do not use.
// Returns an error if installing the capability would fail.
func (a *AlertingNodeBackend) CanInstall(_ context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "Alerting capability not yet supported")
}

// Installs the capability on a cluster.
func (a *AlertingNodeBackend) Install(_ context.Context, _ *capabilityv1.InstallRequest) (*capabilityv1.InstallResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Alerting capability not yet supported")
}

// Returns common runtime config info for this capability from a specific
// cluster (node).
func (a *AlertingNodeBackend) Status(_ context.Context, _ *corev1.Reference) (*capabilityv1.NodeCapabilityStatus, error) {
	return nil, status.Error(codes.Unimplemented, "Alerting capability not yet supported")
}

// Requests the backend to clean up any resources it owns and prepare
// for uninstallation. This process is asynchronous. The status of the
// operation can be queried using the UninstallStatus method, or canceled
// using the CancelUninstall method.
func (a *AlertingNodeBackend) Uninstall(_ context.Context, _ *capabilityv1.UninstallRequest) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "Alerting capability not yet supported")
}

// Gets the status of the uninstall task for the given cluster.
func (a *AlertingNodeBackend) UninstallStatus(_ context.Context, _ *corev1.Reference) (*corev1.TaskStatus, error) {
	return nil, status.Error(codes.Unimplemented, "Alerting capability not yet supported")
}

// Cancels an uninstall task for the given cluster, if it is still pending.
func (a *AlertingNodeBackend) CancelUninstall(_ context.Context, _ *corev1.Reference) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "Alerting capability not yet supported")
}

// Deprecated: Do not use.
// Returns a go template string which will generate a shell command used to
// install the capability. This will be displayed to the user in the UI.
// See InstallerTemplateSpec above for the available template fields.
func (a *AlertingNodeBackend) InstallerTemplate(_ context.Context, _ *emptypb.Empty) (*capabilityv1.InstallerTemplateResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Deprecated API: Do not use")
}
