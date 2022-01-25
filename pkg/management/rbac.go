package management

import (
	context "context"

	core "github.com/kralicky/opni-monitoring/pkg/core"
)

func (s *Server) CreateRole(ctx context.Context, in *core.Role) error {
	return s.rbacStore.CreateRole(ctx, in)
}
func (s *Server) DeleteRole(ctx context.Context, in *core.Reference) error {
	return s.rbacStore.DeleteRole(ctx, in)
}
func (s *Server) GetRole(ctx context.Context, in *core.Reference) (*core.Role, error) {
	return s.rbacStore.GetRole(ctx, in)
}
func (s *Server) CreateRoleBinding(ctx context.Context, in *core.RoleBinding) error {
	return s.rbacStore.CreateRoleBinding(ctx, in)
}
func (s *Server) DeleteRoleBinding(ctx context.Context, in *core.Reference) error {
	return s.rbacStore.DeleteRoleBinding(ctx, in)
}
func (s *Server) GetRoleBinding(ctx context.Context, in *core.Reference) (*core.RoleBinding, error) {
	return s.rbacStore.GetRoleBinding(ctx, in)
}
func (s *Server) ListRoles(ctx context.Context) (*core.RoleList, error) {
	return s.rbacStore.ListRoles(ctx)
}
func (s *Server) ListRoleBindings(ctx context.Context) (*core.RoleBindingList, error) {
	return s.rbacStore.ListRoleBindings(ctx)
}
