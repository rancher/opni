package etcd

import (
	"context"
	"fmt"
	"path"
	"strconv"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
	"k8s.io/client-go/util/retry"
)

func (e *EtcdStore) CreateCluster(ctx context.Context, cluster *corev1.Cluster) error {
	cluster.SetResourceVersion("")
	data, err := protojson.Marshal(cluster)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster: %w", err)
	}
	ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
	defer ca()
	resp, err := e.Client.Put(ctx, path.Join(e.Prefix, clusterKey, cluster.Id), string(data))
	if err != nil {
		return fmt.Errorf("failed to create cluster: %w", err)
	}
	cluster.SetResourceVersion(fmt.Sprint(resp.Header.Revision))
	return nil
}

func (e *EtcdStore) DeleteCluster(ctx context.Context, ref *corev1.Reference) error {
	ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
	defer ca()
	resp, err := e.Client.Delete(ctx, path.Join(e.Prefix, clusterKey, ref.Id))
	if err != nil {
		return fmt.Errorf("failed to delete cluster: %w", err)
	}
	if resp.Deleted == 0 {
		return storage.ErrNotFound
	}
	return nil
}

func (e *EtcdStore) ListClusters(
	ctx context.Context,
	matchLabels *corev1.LabelSelector,
	matchOptions corev1.MatchOptions,
) (*corev1.ClusterList, error) {
	ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
	defer ca()
	resp, err := e.Client.Get(ctx, path.Join(e.Prefix, clusterKey),
		clientv3.WithPrefix(),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %w", err)
	}
	clusters := &corev1.ClusterList{
		Items: []*corev1.Cluster{},
	}
	selectorPredicate := storage.ClusterSelector{
		LabelSelector: matchLabels,
		MatchOptions:  matchOptions,
	}.Predicate()

	for _, kv := range resp.Kvs {
		cluster := &corev1.Cluster{}
		if err := protojson.Unmarshal(kv.Value, cluster); err != nil {
			return nil, fmt.Errorf("failed to unmarshal cluster: %w", err)
		}
		if selectorPredicate(cluster) {
			cluster.SetResourceVersion(fmt.Sprint(kv.ModRevision))
			clusters.Items = append(clusters.Items, cluster)
		}
	}
	return clusters, nil
}

func (e *EtcdStore) GetCluster(ctx context.Context, ref *corev1.Reference) (*corev1.Cluster, error) {
	ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
	defer ca()
	resp, err := e.Client.Get(ctx, path.Join(e.Prefix, clusterKey, ref.Id))
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, storage.ErrNotFound
	}
	cluster := &corev1.Cluster{}
	if err := protojson.Unmarshal(resp.Kvs[0].Value, cluster); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster: %w", err)
	}
	cluster.SetResourceVersion(fmt.Sprint(resp.Kvs[0].ModRevision))
	return cluster, nil
}

func (e *EtcdStore) UpdateCluster(
	ctx context.Context,
	ref *corev1.Reference,
	mutator storage.MutatorFunc[*corev1.Cluster],
) (*corev1.Cluster, error) {
	var retCluster *corev1.Cluster
	err := retry.OnError(defaultBackoff, isRetryErr, func() error {
		ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
		defer ca()
		txn := e.Client.Txn(ctx)
		key := path.Join(e.Prefix, clusterKey, ref.Id)
		cluster, err := e.GetCluster(ctx, ref)
		if err != nil {
			return fmt.Errorf("failed to get cluster: %w", err)
		}
		revision, err := strconv.Atoi(cluster.GetResourceVersion())
		if err != nil {
			return fmt.Errorf("internal error: cluster has invalid resource version: %w", err)
		}
		mutator(cluster)
		cluster.SetResourceVersion("")
		data, err := protojson.Marshal(cluster)
		if err != nil {
			return fmt.Errorf("failed to marshal cluster: %w", err)
		}
		txnResp, err := txn.If(clientv3.Compare(clientv3.ModRevision(key), "=", revision)).
			Then(clientv3.OpPut(key, string(data))).
			Commit()
		if err != nil {
			e.Logger.With(
				zap.Error(err),
			).Error("error updating cluster")
			return err
		}
		if !txnResp.Succeeded {
			return retryErr
		}
		retCluster = cluster
		return nil
	})
	if err != nil {
		return nil, err
	}
	return retCluster, nil
}

func (e *EtcdStore) WatchCluster(
	ctx context.Context,
	cluster *corev1.Cluster,
) (<-chan storage.WatchEvent[*corev1.Cluster], error) {
	eventC := make(chan storage.WatchEvent[*corev1.Cluster], 10)
	version, err := strconv.ParseInt(cluster.GetResourceVersion(), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse resource version: %w", err)
	}
	wc := e.Client.Watch(ctx, path.Join(e.Prefix, clusterKey, cluster.Id),
		clientv3.WithPrevKV(),
		clientv3.WithRev(version),
	)
	go func() {
		defer close(eventC)
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-wc:
				if event.Err() != nil {
					e.Logger.With(
						zap.Error(event.Err()),
					).Error("error watching cluster")
					return
				}
				for _, ev := range event.Events {
					var eventType storage.WatchEventType
					current := &corev1.Cluster{}
					previous := &corev1.Cluster{}
					switch ev.Type {
					case mvccpb.DELETE:
						eventType = storage.WatchEventDelete
					case mvccpb.PUT:
						eventType = storage.WatchEventPut
					default:
						continue
					}
					if ev.Kv.Version == 0 {
						// deleted
						current = nil
					} else {
						if err := protojson.Unmarshal(ev.Kv.Value, current); err != nil {
							e.Logger.With(
								zap.Error(err),
							).Error("error unmarshaling cluster")
							continue
						}
					}
					if err := protojson.Unmarshal(ev.PrevKv.Value, previous); err != nil {
						e.Logger.With(
							zap.Error(err),
						).Error("error unmarshaling cluster")
						continue
					}
					eventC <- storage.WatchEvent[*corev1.Cluster]{
						EventType: eventType,
						Current:   current,
						Previous:  previous,
					}
				}
			}
		}
	}()
	return eventC, nil
}
