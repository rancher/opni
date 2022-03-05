package management

import (
	"context"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/validation"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) CreateRole(ctx context.Context, in *core.Role) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.storageBackend.CreateRole(ctx, in)
}

func (s *Server) DeleteRole(ctx context.Context, in *core.Reference) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.storageBackend.DeleteRole(ctx, in)
}

func (s *Server) GetRole(ctx context.Context, in *core.Reference) (*core.Role, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	role, err := s.storageBackend.GetRole(ctx, in)
	return role, err
}

func (s *Server) CreateRoleBinding(ctx context.Context, in *core.RoleBinding) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	if len(in.Taints) > 0 {
		return nil, validation.ErrReadOnlyField
	}
	return &emptypb.Empty{}, s.storageBackend.CreateRoleBinding(ctx, in)
}

func (s *Server) DeleteRoleBinding(ctx context.Context, in *core.Reference) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.storageBackend.DeleteRoleBinding(ctx, in)
}

func (s *Server) GetRoleBinding(ctx context.Context, in *core.Reference) (*core.RoleBinding, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	rb, err := s.storageBackend.GetRoleBinding(ctx, in)
	return rb, err
}

func (s *Server) ListRoles(ctx context.Context, _ *emptypb.Empty) (*core.RoleList, error) {
	rl, err := s.storageBackend.ListRoles(ctx)
	return rl, err
}

func (s *Server) ListRoleBindings(ctx context.Context, _ *emptypb.Empty) (*core.RoleBindingList, error) {
	rbl, err := s.storageBackend.ListRoleBindings(ctx)
	return rbl, err
}

func (s *Server) SubjectAccess(ctx context.Context, sar *core.SubjectAccessRequest) (*core.ReferenceList, error) {
	if err := validation.Validate(sar); err != nil {
		return nil, err
	}
	rl, err := s.rbacProvider.SubjectAccess(ctx, sar)
	return rl, err
}
