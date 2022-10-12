package crds

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitoringv1beta1 "github.com/rancher/opni/apis/monitoring/v1beta1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
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
	cluster.SetResourceVersion("")
	return c.client.Create(ctx, &monitoringv1beta1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Id,
			Namespace: c.namespace,
			Labels:    cluster.GetMetadata().GetLabels(),
		},
		Spec: cluster,
	})
}

func (c *CRDStore) DeleteCluster(ctx context.Context, ref *corev1.Reference) error {
	return c.client.Delete(ctx, &monitoringv1beta1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.Id,
			Namespace: c.namespace,
		},
	})
}

func (c *CRDStore) GetCluster(ctx context.Context, ref *corev1.Reference) (*corev1.Cluster, error) {
	cluster := &monitoringv1beta1.Cluster{}
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
	cluster.Spec.SetResourceVersion(cluster.GetResourceVersion())
	return cluster.Spec, nil
}

func (c *CRDStore) UpdateCluster(ctx context.Context, ref *corev1.Reference, mutator storage.MutatorFunc[*corev1.Cluster]) (*corev1.Cluster, error) {
	err := retry.OnError(defaultBackoff, k8serrors.IsConflict, func() error {
		existing := &monitoringv1beta1.Cluster{}
		err := c.client.Get(ctx, client.ObjectKey{
			Name:      ref.Id,
			Namespace: c.namespace,
		}, existing)
		if err != nil {
			return err
		}
		clone := existing.DeepCopy()
		mutator(clone.Spec)
		clone.Spec.SetResourceVersion("")
		return c.client.Update(ctx, clone)
	})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return c.GetCluster(ctx, ref)
}

func (c *CRDStore) ListClusters(ctx context.Context, matchLabels *corev1.LabelSelector, matchOptions corev1.MatchOptions) (*corev1.ClusterList, error) {
	list := &monitoringv1beta1.ClusterList{}
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
			item.Spec.SetResourceVersion(item.GetResourceVersion())
			clusters.Items = append(clusters.Items, item.Spec)
		}
	}
	return clusters, nil
}

func (c *CRDStore) WatchCluster(
	ctx context.Context,
	ref *corev1.Cluster,
) (<-chan storage.WatchEvent[*corev1.Cluster], error) {
	watcher, err := c.client.Watch(ctx, &monitoringv1beta1.ClusterList{}, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", ref.Id),
	})
	if err != nil {
		return nil, err
	}
	eventC := make(chan storage.WatchEvent[*corev1.Cluster], 10)

	go func() {
		<-ctx.Done()
		watcher.Stop()
	}()
	go func() {
		existing := util.ProtoClone(ref)
		for event := range watcher.ResultChan() {
			switch event.Type {
			case watch.Added:
				existing = util.ProtoClone(event.Object.(*monitoringv1beta1.Cluster).Spec)
			case watch.Modified:
				current := util.ProtoClone(event.Object.(*monitoringv1beta1.Cluster).Spec)
				eventC <- storage.WatchEvent[*corev1.Cluster]{
					EventType: storage.WatchEventCreate,
					Current:   util.ProtoClone(current),
					Previous:  util.ProtoClone(existing),
				}
				existing = current
			case watch.Deleted:
				eventC <- storage.WatchEvent[*corev1.Cluster]{
					EventType: storage.WatchEventDelete,
					Current:   nil,
					Previous:  util.ProtoClone(existing),
				}
				existing = nil
			default:
				c.logger.Error(fmt.Sprintf("unexpected event type received from watcher: %v", event.Type))
				close(eventC)
				return
			}
		}
	}()

	return eventC, nil
}

func (c *CRDStore) WatchClusters(
	_ context.Context,
	_ []*corev1.Cluster,
) (<-chan storage.WatchEvent[*corev1.Cluster], error) {
	return nil, status.Error(codes.Unimplemented, "WatchClusters is not available when using CRD cluster storage.")
}
