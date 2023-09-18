package backend

import (
	"context"

	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (m *MetricsBackend) GetAvailablePermissions(_ context.Context, _ *emptypb.Empty) (*v1.AvailablePermissions, error) {
	return &v1.AvailablePermissions{
		Items: []*v1.PermissionDescription{
			{
				Type: string(v1.PermissionTypeCluster),
				Verbs: []*v1.PermissionVerb{
					{Verb: string(storage.ClusterVerbGet)},
				},
			},
		},
	}, nil
}

func (m *MetricsBackend) GetRole(ctx context.Context, in *v1.Reference) (*v1.Role, error) {
	m.WaitForInit()
	return m.StorageBackend.GetRole(ctx, in)
}

func (m *MetricsBackend) CreateRole(ctx context.Context, in *v1.Role) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return &emptypb.Empty{}, err
	}

	m.WaitForInit()
	err := m.StorageBackend.CreateRole(ctx, in)
	return &emptypb.Empty{}, err
}

func (m *MetricsBackend) UpdateRole(ctx context.Context, in *v1.Role) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return &emptypb.Empty{}, err
	}

	m.WaitForInit()

	oldRole, err := m.StorageBackend.GetRole(ctx, in.Reference())
	if err != nil {
		return &emptypb.Empty{}, err
	}

	_, err = m.StorageBackend.UpdateRole(ctx, oldRole.Reference(), func(role *v1.Role) {
		role.Permissions = in.GetPermissions()
		role.Metadata = in.GetMetadata()
	})
	return &emptypb.Empty{}, err
}

func (m *MetricsBackend) DeleteRole(ctx context.Context, in *v1.Reference) (*emptypb.Empty, error) {
	m.WaitForInit()
	err := m.StorageBackend.DeleteRole(ctx, in)
	return &emptypb.Empty{}, err
}

func (m *MetricsBackend) ListRoles(ctx context.Context, _ *emptypb.Empty) (*v1.RoleList, error) {
	m.WaitForInit()
	return m.StorageBackend.ListRoles(ctx)
}
