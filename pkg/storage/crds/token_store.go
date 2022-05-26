package crds

import (
	"context"
	"time"

	"github.com/rancher/opni/apis/v1beta2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/tokens"
	"k8s.io/apimachinery/pkg/api/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *CRDStore) CreateToken(ctx context.Context, ttl time.Duration, opts ...storage.TokenCreateOption) (*corev1.BootstrapToken, error) {
	options := storage.NewTokenCreateOptions()
	options.Apply(opts...)

	token := tokens.NewToken().ToBootstrapToken()
	token.Metadata = &corev1.BootstrapTokenMetadata{
		LeaseID:      -1,
		Ttl:          int64(ttl.Seconds()),
		UsageCount:   0,
		Labels:       options.Labels,
		Capabilities: options.Capabilities,
	}
	err := c.client.Create(ctx, &v1beta2.BootstrapToken{
		ObjectMeta: metav1.ObjectMeta{
			Name:      token.TokenID,
			Namespace: c.namespace,
			Labels:    options.Labels,
		},
		Spec: token,
	})
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (c *CRDStore) DeleteToken(ctx context.Context, ref *corev1.Reference) error {
	err := c.client.Delete(ctx, &v1beta2.BootstrapToken{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.Id,
			Namespace: c.namespace,
		},
	})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return storage.ErrNotFound
		}
		return err
	}
	return nil
}

func (c *CRDStore) GetToken(ctx context.Context, ref *corev1.Reference) (*corev1.BootstrapToken, error) {
	token := &v1beta2.BootstrapToken{}
	err := c.client.Get(ctx, client.ObjectKey{
		Name:      ref.Id,
		Namespace: c.namespace,
	}, token)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	patchTTL(token)
	if token.Spec.Metadata.Ttl <= 0 {
		go c.garbageCollectToken(token)
		return nil, errors.NewNotFound(schema.GroupResource{
			Group:    "monitoring.opni.io",
			Resource: "BootstrapToken",
		}, token.GetName())
	}
	return token.Spec, nil
}

func (c *CRDStore) ListTokens(ctx context.Context) ([]*corev1.BootstrapToken, error) {
	list := &v1beta2.BootstrapTokenList{}
	err := c.client.List(ctx, list, client.InNamespace(c.namespace))
	if err != nil {
		return nil, err
	}
	tokens := make([]*corev1.BootstrapToken, len(list.Items))
	for i, item := range list.Items {
		patchTTL(&list.Items[i])
		if item.Spec.Metadata.Ttl <= 0 {
			go c.garbageCollectToken(&list.Items[i])
			continue
		}
		tokens[i] = item.Spec
	}
	return tokens, nil
}

func (c *CRDStore) UpdateToken(ctx context.Context, ref *corev1.Reference, mutator storage.MutatorFunc[*corev1.BootstrapToken]) (*corev1.BootstrapToken, error) {
	var token *corev1.BootstrapToken
	err := retry.OnError(defaultBackoff, k8serrors.IsConflict, func() error {
		existing := &v1beta2.BootstrapToken{}
		err := c.client.Get(ctx, client.ObjectKey{
			Name:      ref.Id,
			Namespace: c.namespace,
		}, existing)
		if err != nil {
			return err
		}
		clone := existing.DeepCopy()
		mutator(clone.Spec)
		token = clone.Spec
		return c.client.Update(ctx, clone)
	})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return token, nil
}

// garbageCollectToken performs a best-effort deletion of an expired token.
func (c *CRDStore) garbageCollectToken(token *v1beta2.BootstrapToken) {
	c.logger.With(
		"token", token.GetName(),
	).Debug("garbage-collecting expired token")
	retry.OnError(retry.DefaultBackoff, func(err error) bool {
		return !errors.IsNotFound(err)
	}, func() error {
		return c.client.Delete(context.Background(), token)
	})
}

func patchTTL(token *v1beta2.BootstrapToken) {
	created := token.ObjectMeta.CreationTimestamp
	ttl := token.Spec.Metadata.Ttl
	// edit the ttl to reflect the current ttl of the token
	newTtl := int64(ttl - (time.Now().Unix() - created.Unix()))
	if newTtl < 0 {
		newTtl = 0
	}
	token.Spec.Metadata.Ttl = newTtl
}
