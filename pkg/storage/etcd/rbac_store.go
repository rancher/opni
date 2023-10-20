package etcd

import (
	"context"
	"fmt"
	"path"
	"strconv"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/lestrrat-go/backoff/v2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
)

func (e *EtcdStore) CreateRole(ctx context.Context, role *corev1.Role) error {
	role.SetResourceVersion("")
	data, err := protojson.Marshal(role)
	if err != nil {
		return fmt.Errorf("failed to marshal role: %w", err)
	}
	key := path.Join(e.Prefix, roleKey, role.Id)
	resp, err := e.Client.Txn(ctx).If(
		clientv3.Compare(clientv3.Version(key), "=", 0),
	).Then(
		clientv3.OpPut(key, string(data)),
	).Commit()
	if err != nil {
		return fmt.Errorf("failed to create role: %w", err)
	}
	if !resp.Succeeded {
		return storage.ErrAlreadyExists
	}
	role.SetResourceVersion(fmt.Sprint(resp.Header.Revision))
	return nil
}

func (e *EtcdStore) UpdateRole(
	ctx context.Context,
	ref *corev1.Reference,
	mutator storage.MutatorFunc[*corev1.Role],
) (*corev1.Role, error) {
	var retRole *corev1.Role

	retryFunc := func() error {
		txn := e.Client.Txn(ctx)
		key := path.Join(e.Prefix, roleKey, ref.Id)
		role, err := e.GetRole(ctx, ref)
		if err != nil {
			return fmt.Errorf("failed to get role: %w", err)
		}
		revision, err := strconv.Atoi(role.GetResourceVersion())
		if err != nil {
			return fmt.Errorf("internal error: role has invalid resource version: %w", err)
		}
		mutator(role)
		data, err := protojson.Marshal(role)
		if err != nil {
			return fmt.Errorf("failed to marshal role: %w", err)
		}
		txnResp, err := txn.If(clientv3.Compare(clientv3.ModRevision(key), "=", revision)).
			Then(clientv3.OpPut(key, string(data))).
			Commit()
		if err != nil {
			e.Logger.With(
				logger.Err(err),
			).Error("error updating role")
			return err
		}
		if !txnResp.Succeeded {
			return errRetry
		}
		role.SetResourceVersion(fmt.Sprint(txnResp.Header.Revision))
		retRole = role
		return nil
	}
	c := defaultBackoff.Start(ctx)
	var err error
	for backoff.Continue(c) {
		err = retryFunc()
		if isRetryErr(err) {
			continue
		}
		break
	}
	if err != nil {
		return nil, err
	}
	return retRole, nil
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
	role.SetResourceVersion(fmt.Sprint(resp.Kvs[0].ModRevision))
	return role, nil
}

func (e *EtcdStore) CreateRoleBinding(ctx context.Context, roleBinding *corev1.RoleBinding) error {
	roleBinding.SetResourceVersion("")
	data, err := protojson.Marshal(roleBinding)
	if err != nil {
		return fmt.Errorf("failed to marshal role binding: %w", err)
	}
	key := path.Join(e.Prefix, roleBindingKey, roleBinding.Id)
	resp, err := e.Client.Txn(ctx).If(
		clientv3.Compare(clientv3.Version(key), "=", 0),
	).Then(
		clientv3.OpPut(key, string(data)),
	).Commit()
	if err != nil {
		return fmt.Errorf("failed to create role binding: %w", err)
	}
	if !resp.Succeeded {
		return storage.ErrAlreadyExists
	}
	roleBinding.SetResourceVersion(fmt.Sprint(resp.Header.Revision))
	return nil
}

func (e *EtcdStore) UpdateRoleBinding(
	ctx context.Context,
	ref *corev1.Reference,
	mutator storage.MutatorFunc[*corev1.RoleBinding],
) (*corev1.RoleBinding, error) {
	var retRb *corev1.RoleBinding

	retryFunc := func() error {
		txn := e.Client.Txn(ctx)
		key := path.Join(e.Prefix, roleBindingKey, ref.Id)
		rb, err := e.GetRoleBinding(ctx, ref)
		if err != nil {
			return fmt.Errorf("failed to get role binding: %w", err)
		}
		revision, err := strconv.Atoi(rb.GetResourceVersion())
		if err != nil {
			return fmt.Errorf("internal error: role binding has invalid resource version: %w", err)
		}
		mutator(rb)
		data, err := protojson.Marshal(rb)
		if err != nil {
			return fmt.Errorf("failed to marshal role binding: %w", err)
		}
		txnResp, err := txn.If(clientv3.Compare(clientv3.ModRevision(key), "=", revision)).
			Then(clientv3.OpPut(key, string(data))).
			Commit()
		if err != nil {
			e.Logger.With(
				logger.Err(err),
			).Error("error updating role binding")
			return err
		}
		if !txnResp.Succeeded {
			return errRetry
		}
		rb.SetResourceVersion(fmt.Sprint(txnResp.Header.Revision))
		retRb = rb
		return nil
	}
	c := defaultBackoff.Start(ctx)
	var err error
	for backoff.Continue(c) {
		err = retryFunc()
		if isRetryErr(err) {
			continue
		}
		break
	}
	if err != nil {
		return nil, err
	}
	return retRb, nil
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
	roleBinding.SetResourceVersion(fmt.Sprint(resp.Kvs[0].ModRevision))
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
