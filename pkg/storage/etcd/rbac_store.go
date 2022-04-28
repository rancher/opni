package etcd

import (
	"context"
	"fmt"
	"path"

	"github.com/rancher/opni/pkg/core"
	"github.com/rancher/opni/pkg/storage"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/encoding/protojson"
)

func (e *EtcdStore) CreateRole(ctx context.Context, role *core.Role) error {
	ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
	defer ca()
	data, err := protojson.Marshal(role)
	if err != nil {
		return fmt.Errorf("failed to marshal role: %w", err)
	}
	_, err = e.Client.Put(ctx, path.Join(e.Prefix, roleKey, role.Id), string(data))
	if err != nil {
		return fmt.Errorf("failed to create role: %w", err)
	}
	return nil
}

func (e *EtcdStore) DeleteRole(ctx context.Context, ref *core.Reference) error {
	ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
	defer ca()
	resp, err := e.Client.Delete(ctx, path.Join(e.Prefix, roleKey, ref.Id))
	if err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}
	if resp.Deleted == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (e *EtcdStore) GetRole(ctx context.Context, ref *core.Reference) (*core.Role, error) {
	ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
	defer ca()
	resp, err := e.Client.Get(ctx, path.Join(e.Prefix, roleKey, ref.Id))
	if err != nil {
		return nil, fmt.Errorf("failed to get role: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("failed to get role: %w", storage.ErrNotFound)
	}
	role := &core.Role{}
	if err := protojson.Unmarshal(resp.Kvs[0].Value, role); err != nil {
		return nil, fmt.Errorf("failed to unmarshal role: %w", err)
	}
	return role, nil
}

func (e *EtcdStore) CreateRoleBinding(ctx context.Context, roleBinding *core.RoleBinding) error {
	ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
	defer ca()
	data, err := protojson.Marshal(roleBinding)
	if err != nil {
		return fmt.Errorf("failed to marshal role binding: %w", err)
	}
	_, err = e.Client.Put(ctx, path.Join(e.Prefix, roleBindingKey, roleBinding.Id), string(data))
	if err != nil {
		return fmt.Errorf("failed to create role binding: %w", err)
	}
	return nil
}

func (e *EtcdStore) DeleteRoleBinding(ctx context.Context, ref *core.Reference) error {
	ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
	defer ca()
	resp, err := e.Client.Delete(ctx, path.Join(e.Prefix, roleBindingKey, ref.Id))
	if err != nil {
		return fmt.Errorf("failed to delete role binding: %w", err)
	}
	if resp.Deleted == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (e *EtcdStore) GetRoleBinding(ctx context.Context, ref *core.Reference) (*core.RoleBinding, error) {
	ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
	defer ca()
	resp, err := e.Client.Get(ctx, path.Join(e.Prefix, roleBindingKey, ref.Id))
	if err != nil {
		return nil, fmt.Errorf("failed to get role binding: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("failed to get role binding: %w", storage.ErrNotFound)
	}
	roleBinding := &core.RoleBinding{}
	if err := protojson.Unmarshal(resp.Kvs[0].Value, roleBinding); err != nil {
		return nil, fmt.Errorf("failed to unmarshal role binding: %w", err)
	}
	if err := storage.ApplyRoleBindingTaints(ctx, e, roleBinding); err != nil {
		return nil, err
	}
	return roleBinding, nil
}

func (e *EtcdStore) ListRoles(ctx context.Context) (*core.RoleList, error) {
	ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
	defer ca()
	resp, err := e.Client.Get(ctx, path.Join(e.Prefix, roleKey), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}
	roleList := &core.RoleList{
		Items: make([]*core.Role, len(resp.Kvs)),
	}
	for i, kv := range resp.Kvs {
		role := &core.Role{}
		if err := protojson.Unmarshal(kv.Value, role); err != nil {
			return nil, fmt.Errorf("failed to unmarshal role: %w", err)
		}
		roleList.Items[i] = role
	}
	return roleList, nil
}

func (e *EtcdStore) ListRoleBindings(ctx context.Context) (*core.RoleBindingList, error) {
	ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
	defer ca()
	resp, err := e.Client.Get(ctx, path.Join(e.Prefix, roleBindingKey), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list role bindings: %w", err)
	}
	roleBindingList := &core.RoleBindingList{
		Items: make([]*core.RoleBinding, len(resp.Kvs)),
	}
	for i, kv := range resp.Kvs {
		roleBinding := &core.RoleBinding{}
		if err := protojson.Unmarshal(kv.Value, roleBinding); err != nil {
			return nil, fmt.Errorf("failed to decode role binding: %w", err)
		}
		if err := storage.ApplyRoleBindingTaints(ctx, e, roleBinding); err != nil {
			return nil, err
		}
		roleBindingList.Items[i] = roleBinding
	}
	return roleBindingList, nil
}
