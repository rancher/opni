package jetstream

import (
	"context"
	"errors"
	"strings"

	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/pkg/storage"
	"github.com/samber/lo"
)

type jetstreamKeyValueStore struct {
	kv nats.KeyValue
}

func (j jetstreamKeyValueStore) Put(ctx context.Context, key string, value []byte) error {
	_, err := j.kv.Put(key, value)
	return err
}

func (j jetstreamKeyValueStore) Get(ctx context.Context, key string) ([]byte, error) {
	resp, err := j.kv.Get(key)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return resp.Value(), nil
}

func (j jetstreamKeyValueStore) Delete(ctx context.Context, key string) error {
	if _, err := j.kv.Get(key); err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return storage.ErrNotFound
		}
		return err
	}
	return j.kv.Delete(key)
}

func (j jetstreamKeyValueStore) ListKeys(ctx context.Context, prefix string) ([]string, error) {
	keys, err := j.kv.Keys()
	if err != nil {
		if errors.Is(err, nats.ErrNoKeysFound) {
			return []string{}, nil
		}
		return nil, err
	}
	return lo.Filter(keys, func(key string, _ int) bool {
		return strings.HasPrefix(key, prefix)
	}), nil
}
