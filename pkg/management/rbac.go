package management

import (
	"context"
	"errors"
	"slices"

	"github.com/gin-gonic/gin"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/proxy"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/validation"
	"github.com/samber/lo"
	ginoauth2 "github.com/zalando/gin-oauth2"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	adminRoleBindingName = "OPNI_admin"
	managmentCapability  = "mgmt"
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
	if err := validation.Validate(in.GetRoleRef()); err != nil {
		return nil, err
	}
	client, err := s.rbacManagerStore.Get(in.GetCapability().GetName())
	if err != nil {
		return nil, err
	}
	return client.DeleteRole(ctx, in.GetRoleRef())
}

func (s *Server) GetBackendRole(ctx context.Context, in *corev1.BackendRoleRequest) (*corev1.Role, error) {
	if err := validation.Validate(in.GetRoleRef()); err != nil {
		return nil, err
	}
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
	return client.ListRoles(ctx, &emptypb.Empty{})
}

func (s *Server) AddAdminRoleBinding(ctx context.Context, in *corev1.Reference) (*emptypb.Empty, error) {
	rbRef := &corev1.Reference{
		Id: adminRoleBindingName,
	}
	_, err := s.coreDataSource.StorageBackend().GetRoleBinding(ctx, rbRef)

	if err == nil {
		_, err := s.coreDataSource.StorageBackend().UpdateRoleBinding(ctx, rbRef, func(rb *corev1.RoleBinding) {
			rb.Subjects = append(rb.Subjects, in.GetId())
		})
		return &emptypb.Empty{}, err
	}

	if errors.Is(err, storage.ErrNotFound) {
		err := s.coreDataSource.StorageBackend().CreateRoleBinding(ctx, &corev1.RoleBinding{
			Id:       adminRoleBindingName,
			RoleId:   "admin",
			Subjects: []string{in.GetId()},
			Metadata: &corev1.RoleBindingMetadata{
				Capability: lo.ToPtr(managmentCapability),
			},
		})
		return &emptypb.Empty{}, err
	}

	return nil, err
}

func (s *Server) RemoveAdminRoleBinding(ctx context.Context, in *corev1.Reference) (*emptypb.Empty, error) {
	_, err := s.coreDataSource.StorageBackend().UpdateRoleBinding(ctx, &corev1.Reference{
		Id: adminRoleBindingName,
	}, func(rb *corev1.RoleBinding) {
		var index int
		for i, subject := range rb.GetSubjects() {
			if subject == in.GetId() {
				index = i
				break
			}
		}
		rb.Subjects = slices.Delete(rb.GetSubjects(), index, index+1)
	})
	return &emptypb.Empty{}, err
}

func (s *Server) ListAdminRoleBinding(ctx context.Context, _ *emptypb.Empty) (*corev1.ReferenceList, error) {
	rb, err := s.coreDataSource.StorageBackend().GetRoleBinding(ctx, &corev1.Reference{
		Id: adminRoleBindingName,
	})
	if err != nil {
		return nil, err
	}
	refs := make([]*corev1.Reference, len(rb.GetSubjects()))
	for i, subject := range rb.GetSubjects() {
		refs[i] = &corev1.Reference{
			Id: subject,
		}
	}
	return &corev1.ReferenceList{
		Items: refs,
	}, nil
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

func (s *Server) ListRoleBindings(ctx context.Context, _ *emptypb.Empty) (*corev1.RoleBindingList, error) {
	rbl, err := s.coreDataSource.StorageBackend().ListRoleBindings(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*corev1.RoleBinding, 0, len(rbl.GetItems()))
	for _, item := range rbl.GetItems() {
		if item.GetMetadata().GetCapability() != managmentCapability {
			items = append(items, item)
		}
	}
	rbl.Items = items
	return rbl, nil
}

func (s *Server) checkAdminAccess(_ *ginoauth2.TokenContainer, ctx *gin.Context) bool {
	lg := s.logger.WithGroup("auth")
	user, ok := ctx.Get(proxy.SubjectKey)
	if !ok {
		lg.Warn("no user in context")
		return false
	}
	userID, ok := user.(string)
	if !ok {
		lg.Warn("could not find user string in context")
		return false
	}

	if userID == "OPNI_admin" {
		return true
	}

	rb, err := s.coreDataSource.StorageBackend().GetRoleBinding(ctx, &corev1.Reference{
		Id: adminRoleBindingName,
	})
	if err != nil {
		lg.With(logger.Err(err)).Error("failed to fetch admin user list")
	}

	for _, subject := range rb.Subjects {
		if userID == subject {
			return true
		}
	}
	return false
}
