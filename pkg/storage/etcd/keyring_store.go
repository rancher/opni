package etcd

import (
	"context"
	"fmt"
	"path"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/keyring"
	"github.com/rancher/opni-monitoring/pkg/storage"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type etcdKeyringStore struct {
	client    *clientv3.Client
	ref       *core.Reference
	prefix    string
	namespace string
}

func (ks *etcdKeyringStore) Put(ctx context.Context, keyring keyring.Keyring) error {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	k, err := keyring.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal keyring: %w", err)
	}
	_, err = ks.client.Put(ctx, path.Join(ks.namespace, keyringKey, ks.prefix, ks.ref.Id), string(k))
	if err != nil {
		return fmt.Errorf("failed to put keyring: %w", err)
	}
	return nil
}

func (ks *etcdKeyringStore) Get(ctx context.Context) (keyring.Keyring, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := ks.client.Get(ctx, path.Join(ks.namespace, keyringKey, ks.prefix, ks.ref.Id))
	if err != nil {
		return nil, fmt.Errorf("failed to get keyring: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, storage.ErrNotFound
	}
	k, err := keyring.Unmarshal(resp.Kvs[0].Value)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal keyring: %w", err)
	}
	return k, nil
}
