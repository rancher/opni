package management

import (
	"context"

	"github.com/rancher/opni/pkg/core"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) CreateRole(ctx context.Context, in *core.Role) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.coreDataSource.StorageBackend().CreateRole(ctx, in)
}

func (s *Server) DeleteRole(ctx context.Context, in *core.Reference) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.coreDataSource.StorageBackend().DeleteRole(ctx, in)
}

func (s *Server) GetRole(ctx context.Context, in *core.Reference) (*core.Role, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	role, err := s.coreDataSource.StorageBackend().GetRole(ctx, in)
	return role, err
}

func (s *Server) CreateRoleBinding(ctx context.Context, in *core.RoleBinding) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	if len(in.Taints) > 0 {
		return nil, validation.ErrReadOnlyField
	}
	return &emptypb.Empty{}, s.coreDataSource.StorageBackend().CreateRoleBinding(ctx, in)
}

func (s *Server) DeleteRoleBinding(ctx context.Context, in *core.Reference) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.coreDataSource.StorageBackend().DeleteRoleBinding(ctx, in)
}

func (s *Server) GetRoleBinding(ctx context.Context, in *core.Reference) (*core.RoleBinding, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	rb, err := s.coreDataSource.StorageBackend().GetRoleBinding(ctx, in)
	return rb, err
}

func (s *Server) ListRoles(ctx context.Context, _ *emptypb.Empty) (*core.RoleList, error) {
	rl, err := s.coreDataSource.StorageBackend().ListRoles(ctx)
	return rl, err
}

func (s *Server) ListRoleBindings(ctx context.Context, _ *emptypb.Empty) (*core.RoleBindingList, error) {
	rbl, err := s.coreDataSource.StorageBackend().ListRoleBindings(ctx)
	return rbl, err
}

func (s *Server) SubjectAccess(ctx context.Context, sar *core.SubjectAccessRequest) (*core.ReferenceList, error) {
	if err := validation.Validate(sar); err != nil {
		return nil, err
	}
	rl, err := s.rbacProvider.SubjectAccess(ctx, sar)
	return rl, err
}
