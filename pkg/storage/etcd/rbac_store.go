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
	roleBinding.SetResourceVersion(fmt.Sprint(resp.Kvs[0].ModRevision))
	return roleBinding, nil
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
		roleBindingList.Items[i] = roleBinding
	}
	return roleBindingList, nil
}
