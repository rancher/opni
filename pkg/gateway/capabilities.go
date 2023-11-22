package gateway

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/management"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type capabilitiesDataSource struct {
	logger          *slog.Logger
	capBackendStore capabilities.BackendStore
	delegate        *DelegateServer
}

type targetedRequest interface {
	GetAgent() *corev1.Reference
	GetCapability() *corev1.Reference
}

func routeManagementRequest[T targetedRequest, R proto.Message](
	s *capabilitiesDataSource, ctx context.Context, in T, //nolint:revive
	backendFunc func(capabilityv1.BackendClient, context.Context, T, ...grpc.CallOption) (R, error),
	managementFunc func(managementv1.ManagementClient, context.Context, T, ...grpc.CallOption) (R, error),
) (R, error) {
	const delegateIdKey = "delegate-id"
	idMeta := metadata.ValueFromIncomingContext(ctx, delegateIdKey)
	s.logger.With("localId", s.delegate.uuid, "peerId", idMeta, "agent", in.GetAgent()).Debug("routing management request")
	client, err := s.delegate.NewTargetedManagementClient(in.GetAgent())
	if err != nil {
		if errors.Is(err, ErrLocalTarget) {
			backendClient, err := s.capBackendStore.Get(in.GetCapability().GetId())
			if err != nil {
				return lo.Empty[R](), err
			}
			return backendFunc(backendClient, ctx, in)
		}
		return lo.Empty[R](), err
	}

	if slices.Contains(idMeta, s.delegate.uuid) {
		return lo.Empty[R](), status.Errorf(codes.Internal, "delegate routing loop detected")
	}
	ctx = metadata.AppendToOutgoingContext(ctx, delegateIdKey, s.delegate.uuid)
	return managementFunc(client, ctx, in)
}

// Info implements management.CapabilitiesDataSource.
func (s *capabilitiesDataSource) Info(ctx context.Context, in *corev1.Reference) (*capabilityv1.Details, error) {
	client, err := s.capBackendStore.Get(in.GetId())
	if err != nil {
		return nil, err
	}
	info, err := client.Info(ctx, in)
	if err != nil {
		return nil, err
	}
	return info, nil
}

// List implements management.CapabilitiesDataSource.
func (s *capabilitiesDataSource) List(ctx context.Context, _ *emptypb.Empty) (*capabilityv1.DetailsList, error) {
	names := s.capBackendStore.List()
	list := &capabilityv1.DetailsList{}
	for _, name := range names {
		client, err := s.capBackendStore.Get(name)
		if err != nil {
			return nil, err
		}
		info, err := client.Info(ctx, &corev1.Reference{Id: name})
		if err != nil {
			return nil, err
		}
		list.Items = append(list.Items, info)
	}
	return list, nil
}

// CancelUninstall implements management.CapabilitiesDataSource.
func (s *capabilitiesDataSource) CancelUninstall(ctx context.Context, in *capabilityv1.CancelUninstallRequest) (*emptypb.Empty, error) {
	return routeManagementRequest(s, ctx, in, capabilityv1.BackendClient.CancelUninstall, managementv1.ManagementClient.CancelCapabilityUninstall)
}

// Install implements management.CapabilitiesDataSource.
func (s *capabilitiesDataSource) Install(ctx context.Context, in *capabilityv1.InstallRequest) (*capabilityv1.InstallResponse, error) {
	return routeManagementRequest(s, ctx, in, capabilityv1.BackendClient.Install, managementv1.ManagementClient.InstallCapability)
}

// Status implements management.CapabilitiesDataSource.
func (s *capabilitiesDataSource) Status(ctx context.Context, in *capabilityv1.StatusRequest) (*capabilityv1.NodeCapabilityStatus, error) {
	return routeManagementRequest(s, ctx, in, capabilityv1.BackendClient.Status, managementv1.ManagementClient.CapabilityStatus)
}

// Uninstall implements management.CapabilitiesDataSource.
func (s *capabilitiesDataSource) Uninstall(ctx context.Context, in *capabilityv1.UninstallRequest) (*emptypb.Empty, error) {
	return routeManagementRequest(s, ctx, in, capabilityv1.BackendClient.Uninstall, managementv1.ManagementClient.UninstallCapability)
}

// UninstallStatus implements management.CapabilitiesDataSource.
func (s *capabilitiesDataSource) UninstallStatus(ctx context.Context, in *capabilityv1.UninstallStatusRequest) (*corev1.TaskStatus, error) {
	return routeManagementRequest(s, ctx, in, capabilityv1.BackendClient.UninstallStatus, managementv1.ManagementClient.CapabilityUninstallStatus)
}

var _ management.CapabilitiesDataSource = (*capabilitiesDataSource)(nil)
