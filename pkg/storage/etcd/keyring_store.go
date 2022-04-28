package etcd

import (
	"context"
	"fmt"
	"path"

	"github.com/rancher/opni/pkg/core"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/storage"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type etcdKeyringStore struct {
	EtcdStoreOptions
	client *clientv3.Client
	ref    *core.Reference
	prefix string
}

func (ks *etcdKeyringStore) Put(ctx context.Context, keyring keyring.Keyring) error {
	ctx, ca := context.WithTimeout(ctx, ks.CommandTimeout)
	defer ca()
	k, err := keyring.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal keyring: %w", err)
	}
	_, err = ks.client.Put(ctx, path.Join(ks.prefix, keyringKey, ks.ref.Id), string(k))
	if err != nil {
		return fmt.Errorf("failed to put keyring: %w", err)
	}
	return nil
}

func (ks *etcdKeyringStore) Get(ctx context.Context) (keyring.Keyring, error) {
	ctx, ca := context.WithTimeout(ctx, ks.CommandTimeout)
	defer ca()
	resp, err := ks.client.Get(ctx, path.Join(ks.prefix, keyringKey, ks.ref.Id))
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
