package etcd

import (
	"context"
	"encoding/base64"
	"path"

	"github.com/rancher/opni-monitoring/pkg/storage"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type genericKeyValueStore struct {
	EtcdStoreOptions
	client *clientv3.Client
	prefix string
}

func (s *genericKeyValueStore) Put(ctx context.Context, key string, value []byte) error {
	ctx, ca := context.WithTimeout(ctx, s.CommandTimeout)
	defer ca()
	_, err := s.client.Put(ctx, path.Join(s.prefix, key), base64.StdEncoding.EncodeToString(value))
	if err != nil {
		return err
	}
	return nil
}

func (s *genericKeyValueStore) Get(ctx context.Context, key string) ([]byte, error) {
	ctx, ca := context.WithTimeout(ctx, s.CommandTimeout)
	defer ca()
	resp, err := s.client.Get(ctx, path.Join(s.prefix, key))
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		return nil, storage.ErrNotFound
	}
	return base64.StdEncoding.DecodeString(string(resp.Kvs[0].Value))
}

func (s *genericKeyValueStore) ListKeys(ctx context.Context, prefix string) ([]string, error) {
	ctx, ca := context.WithTimeout(ctx, s.CommandTimeout)
	defer ca()
	resp, err := s.client.Get(ctx, path.Join(s.prefix, prefix),
		clientv3.WithPrefix(),
		clientv3.WithKeysOnly(),
	)
	if err != nil {
		return nil, err
	}
	keys := make([]string, len(resp.Kvs))
	for i, kv := range resp.Kvs {
		keys[i] = string(kv.Key)
	}
	return keys, nil
}
