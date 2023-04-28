package etcd

import (
	"context"
	"encoding/base64"
	"fmt"
	"path"
	"strings"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/rancher/opni/pkg/storage"
)

type genericKeyValueStore struct {
	client *clientv3.Client
	prefix string
}

func (s *genericKeyValueStore) Put(ctx context.Context, key string, value []byte) error {
	if err := validateKey(key); err != nil {
		return err
	}
	_, err := s.client.Put(ctx, path.Join(s.prefix, key), base64.StdEncoding.EncodeToString(value))
	if err != nil {
		return err
	}
	return nil
}

func (s *genericKeyValueStore) Get(ctx context.Context, key string) ([]byte, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	resp, err := s.client.Get(ctx, path.Join(s.prefix, key))
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		return nil, storage.ErrNotFound
	}
	return base64.StdEncoding.DecodeString(string(resp.Kvs[0].Value))
}

func (s *genericKeyValueStore) Delete(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	resp, err := s.client.Delete(ctx, path.Join(s.prefix, key))
	if err != nil {
		return err
	}
	if resp.Deleted == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (s *genericKeyValueStore) ListKeys(ctx context.Context, prefix string) ([]string, error) {
	resp, err := s.client.Get(ctx, path.Join(s.prefix, prefix),
		clientv3.WithPrefix(),
		clientv3.WithKeysOnly(),
	)
	if err != nil {
		return nil, err
	}
	keys := make([]string, len(resp.Kvs))
	for i, kv := range resp.Kvs {
		keys[i] = strings.TrimPrefix(string(kv.Key), s.prefix+"/")
	}
	return keys, nil
}

func validateKey(key string) error {
	// etcd will check keys, but we need to check if the key is empty ourselves
	// since we always prepend a prefix to the key
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	return nil
}
