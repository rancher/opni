package management

import (
	"context"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) ListRBACBackends(_ context.Context, _ *emptypb.Empty) (*corev1.CapabilityTypeList, error) {
	capabilities := s.rbacManagerStore.List()
	return &corev1.CapabilityTypeList{
		Names: capabilities,
	}, nil
}

func (s *Server) GetAvailableBackendPermissions(ctx context.Context, in *corev1.CapabilityType) (*corev1.AvailablePermissions, error) {
	client, err := s.rbacManagerStore.Get(in.GetName())
	if err != nil {
		return nil, err
	}
	return client.GetAvailablePermissions(ctx, nil)
}

func (s *Server) CreateBackendRole(ctx context.Context, in *corev1.BackendRole) (*emptypb.Empty, error) {
	if err := validation.Validate(in.GetRole()); err != nil {
		return nil, err
	}

	client, err := s.rbacManagerStore.Get(in.GetCapability().GetName())
	if err != nil {
		return nil, err
	}

	return client.CreateRole(ctx, in.GetRole())
}

func (s *Server) UpdateBackendRole(ctx context.Context, in *corev1.BackendRole) (*emptypb.Empty, error) {
	if err := validation.Validate(in.GetRole()); err != nil {
		return nil, err
	}

	client, err := s.rbacManagerStore.Get(in.GetCapability().GetName())
	if err != nil {
		return nil, err
	}

	return client.UpdateRole(ctx, in.GetRole())
}

func (s *Server) DeleteBackendRole(ctx context.Context, in *corev1.BackendRoleRequest) (*emptypb.Empty, error) {
	client, err := s.rbacManagerStore.Get(in.GetCapability().GetName())
	if err != nil {
		return nil, err
	}
	return client.DeleteRole(ctx, in.GetRoleRef())
}

func (s *Server) GetBackendRole(ctx context.Context, in *corev1.BackendRoleRequest) (*corev1.Role, error) {
	client, err := s.rbacManagerStore.Get(in.GetCapability().GetName())
	if err != nil {
		return nil, err
	}
	return client.GetRole(ctx, in.GetRoleRef())
}

func (s *Server) ListBackendRoles(ctx context.Context, in *corev1.CapabilityType) (*corev1.RoleList, error) {
	client, err := s.rbacManagerStore.Get(in.GetName())
	if err != nil {
		return nil, err
	}
	return client.ListRoles(ctx, nil)
}

func (s *Server) CreateRoleBinding(ctx context.Context, in *corev1.RoleBinding) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	if len(in.Taints) > 0 {
		return nil, validation.ErrReadOnlyField
	}
	return &emptypb.Empty{}, s.coreDataSource.StorageBackend().CreateRoleBinding(ctx, in)
}

func (s *Server) UpdateRoleBinding(
	ctx context.Context,
	in *corev1.RoleBinding,
) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return &emptypb.Empty{}, err
	}

	oldRb, err := s.GetRoleBinding(ctx, in.Reference())
	if err != nil {
		return &emptypb.Empty{}, err
	}

	if len(in.Taints) > 0 {
		return nil, validation.ErrReadOnlyField
	}

	_, err = s.coreDataSource.StorageBackend().UpdateRoleBinding(ctx, oldRb.Reference(), func(rb *corev1.RoleBinding) {
		rb.RoleIds = in.GetRoleIds()
		rb.Subject = in.GetSubject()
		rb.Metadata = in.GetMetadata()
	})
	return &emptypb.Empty{}, err
}

func (s *Server) DeleteRoleBinding(ctx context.Context, in *corev1.Reference) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.coreDataSource.StorageBackend().DeleteRoleBinding(ctx, in)
}

func (s *Server) GetRoleBinding(ctx context.Context, in *corev1.Reference) (*corev1.RoleBinding, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	rb, err := s.coreDataSource.StorageBackend().GetRoleBinding(ctx, in)
	return rb, err
}

// func (s *Server) ListRoles(ctx context.Context, _ *emptypb.Empty) (*corev1.RoleList, error) {
// 	rl, err := s.coreDataSource.StorageBackend().ListRoles(ctx)
// 	return rl, err
// }

func (s *Server) ListRoleBindings(ctx context.Context, _ *emptypb.Empty) (*corev1.RoleBindingList, error) {
	rbl, err := s.coreDataSource.StorageBackend().ListRoleBindings(ctx)
	return rbl, err
}

// func (s *Server) SubjectAccess(ctx context.Context, sar *corev1.SubjectAccessRequest) (*corev1.ReferenceList, error) {
// 	if err := validation.Validate(sar); err != nil {
// 		return nil, err
// 	}
// 	rl, err := s.rbacProvider.SubjectAccess(ctx, sar)
// 	return rl, err
// }
