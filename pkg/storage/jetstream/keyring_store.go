package jetstream

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/nats-io/nats.go"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/storage"
)

type jetstreamKeyringStore struct {
	kv     nats.KeyValue
	ref    *corev1.Reference
	prefix string
}

func (ks *jetstreamKeyringStore) Put(ctx context.Context, keyring keyring.Keyring) error {
	k, err := keyring.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal keyring: %w", err)
	}
	_, err = ks.kv.Put(path.Join(ks.prefix, ks.ref.Id), k)
	if err != nil {
		return fmt.Errorf("failed to put keyring: %w", err)
	}
	return nil
}

func (ks *jetstreamKeyringStore) Get(ctx context.Context) (keyring.Keyring, error) {
	resp, err := ks.kv.Get(path.Join(ks.prefix, ks.ref.Id))
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get keyring: %w", err)
	}

	k, err := keyring.Unmarshal(resp.Value())
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal keyring: %w", err)
	}
	return k, nil
}

func (ks *jetstreamKeyringStore) Delete(ctx context.Context) error {
	err := ks.kv.Delete(path.Join(ks.prefix, ks.ref.Id))
	if err != nil {
		return fmt.Errorf("failed to delete keyring: %w", err)
	}
	return nil
}
