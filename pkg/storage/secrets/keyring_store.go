package secrets

import (
	"context"
	"strings"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/keyring"
	"github.com/rancher/opni-monitoring/pkg/storage"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type secretKeyringStore struct {
	client       client.Client
	prefix       string
	k8sNamespace string
	ref          *core.Reference
}

func (k *secretKeyringStore) NamespacedName() client.ObjectKey {
	parts := []string{"keyring",
		strings.Trim(k.prefix, "-"),
		k.ref.Id,
	}
	return client.ObjectKey{
		Name:      strings.Join(parts, "-"),
		Namespace: k.k8sNamespace,
	}
}

func (k *secretKeyringStore) ObjectMeta() metav1.ObjectMeta {
	key := k.NamespacedName()
	return metav1.ObjectMeta{
		Name:      key.Name,
		Namespace: key.Namespace,
	}
}

func (k *secretKeyringStore) Put(ctx context.Context, keyring keyring.Keyring) error {
	data, err := keyring.Marshal()
	if err != nil {
		return err
	}
	secret := &corev1.Secret{
		ObjectMeta: k.ObjectMeta(),
		Data: map[string][]byte{
			"keyring": data,
		},
	}
	return k.client.Create(ctx, secret)
}

func (k *secretKeyringStore) Get(ctx context.Context) (keyring.Keyring, error) {
	secret := &corev1.Secret{}
	err := k.client.Get(ctx, k.NamespacedName(), secret)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return keyring.Unmarshal(secret.Data["keyring"])
}
