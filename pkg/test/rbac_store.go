package test

import (
	"context"
	"sync"

	"github.com/golang/mock/gomock"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/storage"
	mock_storage "github.com/rancher/opni-monitoring/pkg/test/mock/storage"
	"google.golang.org/protobuf/proto"
)

func NewTestRBACStore(ctrl *gomock.Controller) storage.RBACStore {
	mockRBACStore := mock_storage.NewMockRBACStore(ctrl)

	roles := map[string]*core.Role{}
	rbs := map[string]*core.RoleBinding{}
	mu := sync.Mutex{}

	mockRBACStore.EXPECT().
		CreateRole(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, role *core.Role) error {
			mu.Lock()
			defer mu.Unlock()
			roles[role.Name] = role
			return nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		DeleteRole(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *core.Reference) error {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := roles[ref.Name]; !ok {
				return storage.ErrNotFound
			}
			delete(roles, ref.Name)
			return nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		GetRole(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *core.Reference) (*core.Role, error) {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := roles[ref.Name]; !ok {
				return nil, storage.ErrNotFound
			}
			return roles[ref.Name], nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		ListRoles(gomock.Any()).
		DoAndReturn(func(_ context.Context) (*core.RoleList, error) {
			mu.Lock()
			defer mu.Unlock()
			roleList := &core.RoleList{}
			for _, role := range roles {
				roleList.Items = append(roleList.Items, role)
			}
			return roleList, nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		CreateRoleBinding(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, rb *core.RoleBinding) error {
			mu.Lock()
			defer mu.Unlock()
			rbs[rb.Name] = rb
			return nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		DeleteRoleBinding(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *core.Reference) error {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := rbs[ref.Name]; !ok {
				return storage.ErrNotFound
			}
			delete(rbs, ref.Name)
			return nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		GetRoleBinding(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, ref *core.Reference) (*core.RoleBinding, error) {
			mu.Lock()
			if _, ok := rbs[ref.Name]; !ok {
				mu.Unlock()
				return nil, storage.ErrNotFound
			}
			cloned := proto.Clone(rbs[ref.Name]).(*core.RoleBinding)
			mu.Unlock()
			storage.ApplyRoleBindingTaints(ctx, mockRBACStore, cloned)
			return cloned, nil
		}).
		AnyTimes()
	mockRBACStore.EXPECT().
		ListRoleBindings(gomock.Any()).
		DoAndReturn(func(ctx context.Context) (*core.RoleBindingList, error) {
			mu.Lock()
			rbList := &core.RoleBindingList{}
			for _, rb := range rbs {
				cloned := proto.Clone(rb).(*core.RoleBinding)
				rbList.Items = append(rbList.Items, cloned)
			}
			mu.Unlock()
			for _, rb := range rbList.Items {
				storage.ApplyRoleBindingTaints(ctx, mockRBACStore, rb)
			}
			return rbList, nil
		}).
		AnyTimes()
	return mockRBACStore
}
