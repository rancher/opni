package management

import (
	context "context"

	core "github.com/kralicky/opni-monitoring/pkg/core"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

func (s *Server) CreateRole(ctx context.Context, in *core.Role) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, s.rbacStore.CreateRole(ctx, in)
}
func (s *Server) DeleteRole(ctx context.Context, in *core.Reference) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, s.rbacStore.DeleteRole(ctx, in)
}
func (s *Server) GetRole(ctx context.Context, in *core.Reference) (*core.Role, error) {
	return s.rbacStore.GetRole(ctx, in)
}
func (s *Server) CreateRoleBinding(ctx context.Context, in *core.RoleBinding) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, s.rbacStore.CreateRoleBinding(ctx, in)
}
func (s *Server) DeleteRoleBinding(ctx context.Context, in *core.Reference) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, s.rbacStore.DeleteRoleBinding(ctx, in)
}
func (s *Server) GetRoleBinding(ctx context.Context, in *core.Reference) (*core.RoleBinding, error) {
	return s.rbacStore.GetRoleBinding(ctx, in)
}
func (s *Server) ListRoles(ctx context.Context, _ *emptypb.Empty) (*core.RoleList, error) {
	return s.rbacStore.ListRoles(ctx)
}
func (s *Server) ListRoleBindings(ctx context.Context, _ *emptypb.Empty) (*core.RoleBindingList, error) {
	return s.rbacStore.ListRoleBindings(ctx)
}
