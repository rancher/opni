package secrets

import (
	"context"
	"strings"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/storage"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type secretKeyValueStore struct {
	client       client.Client
	prefix       string
	k8sNamespace string
	ref          *core.Reference
}

func (k *secretKeyValueStore) NamespacedName() client.ObjectKey {
	parts := []string{"kvstore",
		strings.Trim(k.prefix, "-"),
		k.ref.Id,
	}
	return client.ObjectKey{
		Name:      strings.Join(parts, "-"),
		Namespace: k.k8sNamespace,
	}
}

func (k *secretKeyValueStore) ObjectMeta() metav1.ObjectMeta {
	key := k.NamespacedName()
	return metav1.ObjectMeta{
		Name:      key.Name,
		Namespace: key.Namespace,
	}
}

func (s *secretKeyValueStore) Put(ctx context.Context, key string, value []byte) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		secret := &corev1.Secret{
			ObjectMeta: s.ObjectMeta(),
		}
		if err := s.client.Get(ctx, s.NamespacedName(), secret); err != nil {
			if errors.IsNotFound(err) {
				secret.Data = map[string][]byte{
					key: value,
				}
				return s.client.Create(ctx, secret)
			}
			return err
		}
		secret.Data[key] = value
		return s.client.Update(ctx, secret)
	})
}

func (s *secretKeyValueStore) Get(ctx context.Context, key string) ([]byte, error) {
	secret := &corev1.Secret{}
	err := s.client.Get(ctx, s.NamespacedName(), secret)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return secret.Data[key], nil
}

func (s *secretKeyValueStore) ListKeys(ctx context.Context, prefix string) ([]string, error) {
	secret := &corev1.Secret{}
	err := s.client.Get(ctx, s.NamespacedName(), secret)
	if err != nil {
		return nil, err
	}
	keys := []string{}
	for k := range secret.Data {
		keys = append(keys, k)
	}
	return keys, nil
}
