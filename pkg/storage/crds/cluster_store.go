package crds

import (
	"context"
	"time"

	"github.com/rancher/opni/apis/v1beta2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
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

func (c *CRDStore) CreateCluster(ctx context.Context, cluster *corev1.Cluster) error {
	return c.client.Create(ctx, &v1beta2.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Id,
			Namespace: c.namespace,
			Labels:    cluster.GetMetadata().GetLabels(),
		},
		Spec: cluster,
	})
}

func (c *CRDStore) DeleteCluster(ctx context.Context, ref *corev1.Reference) error {
	return c.client.Delete(ctx, &v1beta2.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.Id,
			Namespace: c.namespace,
		},
	})
}

func (c *CRDStore) GetCluster(ctx context.Context, ref *corev1.Reference) (*corev1.Cluster, error) {
	cluster := &v1beta2.Cluster{}
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

func (c *CRDStore) UpdateCluster(ctx context.Context, ref *corev1.Reference, mutator storage.MutatorFunc[*corev1.Cluster]) (*corev1.Cluster, error) {
	var cluster *corev1.Cluster
	err := retry.OnError(defaultBackoff, k8serrors.IsConflict, func() error {
		existing := &v1beta2.Cluster{}
		err := c.client.Get(ctx, client.ObjectKey{
			Name:      ref.Id,
			Namespace: c.namespace,
		}, existing)
		if err != nil {
			return err
		}
		clone := existing.DeepCopy()
		mutator(clone.Spec)
		cluster = clone.Spec
		return c.client.Update(ctx, clone)
	})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return cluster, nil
}

func (c *CRDStore) ListClusters(ctx context.Context, matchLabels *corev1.LabelSelector, matchOptions corev1.MatchOptions) (*corev1.ClusterList, error) {
	list := &v1beta2.ClusterList{}
	err := c.client.List(ctx, list, client.InNamespace(c.namespace))
	if err != nil {
		return nil, err
	}
	selectorPredicate := storage.ClusterSelector{
		LabelSelector: matchLabels,
		MatchOptions:  matchOptions,
	}.Predicate()
	clusters := &corev1.ClusterList{
		Items: make([]*corev1.Cluster, 0, len(list.Items)),
	}
	for _, item := range list.Items {
		if selectorPredicate(item.Spec) {
			clusters.Items = append(clusters.Items, item.Spec)
		}
	}
	return clusters, nil
}
