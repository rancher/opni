package crds

import (
	"context"
	"time"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/sdk/api/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/tokens"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *CRDStore) CreateToken(ctx context.Context, ttl time.Duration, labels map[string]string) (*core.BootstrapToken, error) {
	token := tokens.NewToken().ToBootstrapToken()
	token.Metadata = &core.BootstrapTokenMetadata{
		LeaseID:    -1,
		Ttl:        -1,
		UsageCount: 0,
		Labels:     labels,
	}
	err := c.client.Create(ctx, &v1beta1.BootstrapToken{
		ObjectMeta: metav1.ObjectMeta{
			Name:      token.TokenID,
			Namespace: c.namespace,
			Labels:    labels,
		},
		Spec: token,
	})
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (c *CRDStore) DeleteToken(ctx context.Context, ref *core.Reference) error {
	return c.client.Delete(ctx, &v1beta1.BootstrapToken{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.Id,
			Namespace: c.namespace,
		},
	})
}

func (c *CRDStore) GetToken(ctx context.Context, ref *core.Reference) (*core.BootstrapToken, error) {
	token := &v1beta1.BootstrapToken{}
	err := c.client.Get(ctx, client.ObjectKey{
		Name:      ref.Id,
		Namespace: c.namespace,
	}, token)
	if err != nil {
		return nil, err
	}
	return token.Spec, nil
}

func (c *CRDStore) ListTokens(ctx context.Context) ([]*core.BootstrapToken, error) {
	list := &v1beta1.BootstrapTokenList{}
	err := c.client.List(ctx, list, client.InNamespace(c.namespace))
	if err != nil {
		return nil, err
	}
	tokens := make([]*core.BootstrapToken, len(list.Items))
	for i, item := range list.Items {
		tokens[i] = item.Spec
	}
	return tokens, nil
}

func (c *CRDStore) IncrementUsageCount(ctx context.Context, ref *core.Reference) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		token, err := c.GetToken(ctx, ref)
		if err != nil {
			return err
		}
		if token.Metadata == nil {
			token.Metadata = &core.BootstrapTokenMetadata{}
		}
		token.Metadata.UsageCount++
		return c.client.Update(ctx, &v1beta1.BootstrapToken{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ref.Id,
				Namespace: c.namespace,
			},
			Spec: token,
		})
	})
}
