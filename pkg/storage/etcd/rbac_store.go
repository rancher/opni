//go:build !noetcd

package etcd

import (
	"context"
	"fmt"
	"path"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/encoding/protojson"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
)

func (e *EtcdStore) CreateRole(ctx context.Context, role *corev1.Role) error {
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

func (e *EtcdStore) DeleteRole(ctx context.Context, ref *corev1.Reference) error {
	resp, err := e.Client.Delete(ctx, path.Join(e.Prefix, roleKey, ref.Id))
	if err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}
	if resp.Deleted == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (e *EtcdStore) GetRole(ctx context.Context, ref *corev1.Reference) (*corev1.Role, error) {
	resp, err := e.Client.Get(ctx, path.Join(e.Prefix, roleKey, ref.Id))
	if err != nil {
		return nil, fmt.Errorf("failed to get role: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, storage.ErrNotFound
	}
	role := &corev1.Role{}
	if err := protojson.Unmarshal(resp.Kvs[0].Value, role); err != nil {
		return nil, fmt.Errorf("failed to unmarshal role: %w", err)
	}
	return role, nil
}

func (e *EtcdStore) CreateRoleBinding(ctx context.Context, roleBinding *corev1.RoleBinding) error {
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

func (e *EtcdStore) DeleteRoleBinding(ctx context.Context, ref *corev1.Reference) error {
	resp, err := e.Client.Delete(ctx, path.Join(e.Prefix, roleBindingKey, ref.Id))
	if err != nil {
		return fmt.Errorf("failed to delete role binding: %w", err)
	}
	if resp.Deleted == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (e *EtcdStore) GetRoleBinding(ctx context.Context, ref *corev1.Reference) (*corev1.RoleBinding, error) {
	resp, err := e.Client.Get(ctx, path.Join(e.Prefix, roleBindingKey, ref.Id))
	if err != nil {
		return nil, fmt.Errorf("failed to get role binding: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, storage.ErrNotFound
	}
	roleBinding := &corev1.RoleBinding{}
	if err := protojson.Unmarshal(resp.Kvs[0].Value, roleBinding); err != nil {
		return nil, fmt.Errorf("failed to unmarshal role binding: %w", err)
	}
	if err := storage.ApplyRoleBindingTaints(ctx, e, roleBinding); err != nil {
		return nil, err
	}
	return roleBinding, nil
}

func (e *EtcdStore) ListRoles(ctx context.Context) (*corev1.RoleList, error) {
	resp, err := e.Client.Get(ctx, path.Join(e.Prefix, roleKey), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}
	roleList := &corev1.RoleList{
		Items: make([]*corev1.Role, len(resp.Kvs)),
	}
	for i, kv := range resp.Kvs {
		role := &corev1.Role{}
		if err := protojson.Unmarshal(kv.Value, role); err != nil {
			return nil, fmt.Errorf("failed to unmarshal role: %w", err)
		}
		roleList.Items[i] = role
	}
	return roleList, nil
}

func (e *EtcdStore) ListRoleBindings(ctx context.Context) (*corev1.RoleBindingList, error) {
	resp, err := e.Client.Get(ctx, path.Join(e.Prefix, roleBindingKey), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list role bindings: %w", err)
	}
	roleBindingList := &corev1.RoleBindingList{
		Items: make([]*corev1.RoleBinding, len(resp.Kvs)),
	}
	for i, kv := range resp.Kvs {
		roleBinding := &corev1.RoleBinding{}
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
