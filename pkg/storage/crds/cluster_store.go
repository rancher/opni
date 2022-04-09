package crds

import (
	"context"
	"time"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/sdk/api/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/storage"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	defaultBackoff = wait.Backoff{
		Steps:    20,
		Duration: 10 * time.Millisecond,
		Cap:      1 * time.Second,
		Factor:   1.5,
		Jitter:   0.1,
	}
)

func (c *CRDStore) CreateCluster(ctx context.Context, cluster *core.Cluster) error {
	return c.client.Create(ctx, &v1beta1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Id,
			Namespace: c.namespace,
			Labels:    cluster.GetMetadata().GetLabels(),
		},
		Spec: cluster,
	})
}

func (c *CRDStore) DeleteCluster(ctx context.Context, ref *core.Reference) error {
	return c.client.Delete(ctx, &v1beta1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.Id,
			Namespace: c.namespace,
		},
	})
}

func (c *CRDStore) GetCluster(ctx context.Context, ref *core.Reference) (*core.Cluster, error) {
	cluster := &v1beta1.Cluster{}
	err := c.client.Get(ctx, client.ObjectKey{
		Name:      ref.Id,
		Namespace: c.namespace,
	}, cluster)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return cluster.Spec, nil
}

func (c *CRDStore) UpdateCluster(ctx context.Context, ref *core.Reference, mutator storage.MutatorFunc[*core.Cluster]) (*core.Cluster, error) {
	var cluster *core.Cluster
	err := retry.OnError(defaultBackoff, k8serrors.IsConflict, func() error {
		existing := &v1beta1.Cluster{}
		err := c.client.Get(ctx, client.ObjectKey{
			Name:      ref.Id,
			Namespace: c.namespace,
		}, existing)
		if err != nil {
			return err
		}
		mutator(existing.Spec)
		cluster = existing.Spec
		return c.client.Update(ctx, existing)
	})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return cluster, nil
}

func (c *CRDStore) ListClusters(ctx context.Context, matchLabels *core.LabelSelector, matchOptions core.MatchOptions) (*core.ClusterList, error) {
	list := &v1beta1.ClusterList{}
	err := c.client.List(ctx, list, client.InNamespace(c.namespace))
	if err != nil {
		return nil, err
	}
	selectorPredicate := storage.ClusterSelector{
		LabelSelector: matchLabels,
		MatchOptions:  matchOptions,
	}.Predicate()
	clusters := &core.ClusterList{
		Items: make([]*core.Cluster, 0, len(list.Items)),
	}
	for _, item := range list.Items {
		if selectorPredicate(item.Spec) {
			clusters.Items = append(clusters.Items, item.Spec)
		}
	}
	return clusters, nil
}
