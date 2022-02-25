package secrets

import (
	"context"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SecretsStore struct {
	client    client.Client
	namespace string
}

func (c *SecretsStore) KeyringStore(ctx context.Context, prefix string, ref *core.Reference) (storage.KeyringStore, error) {
	return &secretKeyringStore{
		client:       c.client,
		prefix:       prefix,
		k8sNamespace: c.namespace,
		ref:          ref,
	}, nil
}

func (c *SecretsStore) KeyValueStore(prefix string) (storage.KeyValueStore, error) {
	return &secretKeyValueStore{
		client:       c.client,
		prefix:       prefix,
		k8sNamespace: c.namespace,
	}, nil
}
