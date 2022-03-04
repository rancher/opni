package crds

import (
	"context"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/sdk/api/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *CRDStore) CreateCluster(ctx context.Context, cluster *core.Cluster) error {
	return c.client.Create(ctx, &v1beta1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Id,
			Namespace: c.namespace,
			Labels:    cluster.Labels,
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
		return nil, err
	}
	return cluster.Spec, nil
}

func (c *CRDStore) UpdateCluster(ctx context.Context, cluster *core.Cluster) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing, err := c.GetCluster(ctx, cluster.Reference())
		if err != nil {
			return err
		}
		existing.Labels = cluster.Labels
		return c.client.Update(ctx, &v1beta1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      existing.Id,
				Namespace: c.namespace,
				Labels:    existing.Labels,
			},
			Spec: existing,
		})
	})
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
