package management

import (
	context "context"

	core "github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/validation"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) CreateRole(ctx context.Context, in *core.Role) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, grpcError(s.rbacStore.CreateRole(ctx, in))
}

func (s *Server) DeleteRole(ctx context.Context, in *core.Reference) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, grpcError(s.rbacStore.DeleteRole(ctx, in))
}

func (s *Server) GetRole(ctx context.Context, in *core.Reference) (*core.Role, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	role, err := s.rbacStore.GetRole(ctx, in)
	return role, grpcError(err)
}

func (s *Server) CreateRoleBinding(ctx context.Context, in *core.RoleBinding) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	if len(in.Taints) > 0 {
		return nil, validation.ErrReadOnlyField
	}
	return &emptypb.Empty{}, grpcError(s.rbacStore.CreateRoleBinding(ctx, in))
}

func (s *Server) DeleteRoleBinding(ctx context.Context, in *core.Reference) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, grpcError(s.rbacStore.DeleteRoleBinding(ctx, in))
}

func (s *Server) GetRoleBinding(ctx context.Context, in *core.Reference) (*core.RoleBinding, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	rb, err := s.rbacStore.GetRoleBinding(ctx, in)
	return rb, grpcError(err)
}

func (s *Server) ListRoles(ctx context.Context, _ *emptypb.Empty) (*core.RoleList, error) {
	rl, err := s.rbacStore.ListRoles(ctx)
	return rl, grpcError(err)
}

func (s *Server) ListRoleBindings(ctx context.Context, _ *emptypb.Empty) (*core.RoleBindingList, error) {
	rbl, err := s.rbacStore.ListRoleBindings(ctx)
	return rbl, grpcError(err)
}

func (s *Server) SubjectAccess(ctx context.Context, sar *core.SubjectAccessRequest) (*core.ReferenceList, error) {
	if err := validation.Validate(sar); err != nil {
		return nil, err
	}
	rl, err := s.rbacProvider.SubjectAccess(ctx, sar)
	return rl, grpcError(err)
}
