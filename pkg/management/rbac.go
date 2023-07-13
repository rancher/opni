package management

import (
	"context"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) CreateRole(ctx context.Context, in *corev1.Role) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.coreDataSource.StorageBackend().CreateRole(ctx, in)
}

func (s *Server) UpdateRole(
	ctx context.Context,
	in *corev1.Role,
) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return &emptypb.Empty{}, err
	}

	oldRole, err := s.GetRole(ctx, in.Reference())
	if err != nil {
		return &emptypb.Empty{}, err
	}

	_, err = s.coreDataSource.StorageBackend().UpdateRole(ctx, oldRole.Reference(), func(role *corev1.Role) {
		role.ClusterIDs = in.GetClusterIDs()
		role.MatchLabels = in.GetMatchLabels()
		role.Metadata = in.GetMetadata()
	})
	return &emptypb.Empty{}, err
}

func (s *Server) DeleteRole(ctx context.Context, in *corev1.Reference) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.coreDataSource.StorageBackend().DeleteRole(ctx, in)
}

func (s *Server) GetRole(ctx context.Context, in *corev1.Reference) (*corev1.Role, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	role, err := s.coreDataSource.StorageBackend().GetRole(ctx, in)
	return role, err
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

	_, err = s.coreDataSource.StorageBackend().UpdateRoleBinding(ctx, oldRb.Reference(), func(rb *corev1.RoleBinding) {
		rb.RoleId = in.GetRoleId()
		rb.Subjects = in.GetSubjects()
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

func (s *Server) ListRoles(ctx context.Context, _ *emptypb.Empty) (*corev1.RoleList, error) {
	rl, err := s.coreDataSource.StorageBackend().ListRoles(ctx)
	return rl, err
}

func (s *Server) ListRoleBindings(ctx context.Context, _ *emptypb.Empty) (*corev1.RoleBindingList, error) {
	rbl, err := s.coreDataSource.StorageBackend().ListRoleBindings(ctx)
	return rbl, err
}

func (s *Server) SubjectAccess(ctx context.Context, sar *corev1.SubjectAccessRequest) (*corev1.ReferenceList, error) {
	if err := validation.Validate(sar); err != nil {
		return nil, err
	}
	rl, err := s.rbacProvider.SubjectAccess(ctx, sar)
	return rl, err
}
