package etcd

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/lestrrat-go/backoff/v2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
)

func (e *EtcdStore) CreateCluster(ctx context.Context, cluster *corev1.Cluster) error {
	cluster.SetResourceVersion("")
	cluster.SetCreationTimestamp(time.Now().Truncate(time.Second))

	data, err := protojson.Marshal(cluster)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster: %w", err)
	}

	key := path.Join(e.Prefix, clusterKey, cluster.Id)
	resp, err := e.Client.Txn(ctx).If(
		clientv3.Compare(clientv3.Version(key), "=", 0),
	).Then(
		clientv3.OpPut(key, string(data)),
	).Commit()
	if err != nil {
		return fmt.Errorf("failed to create cluster: %w", err)
	}
	if !resp.Succeeded {
		return storage.ErrAlreadyExists
	}

	cluster.SetResourceVersion(fmt.Sprint(resp.Header.Revision))
	return nil
}

func (e *EtcdStore) DeleteCluster(ctx context.Context, ref *corev1.Reference) error {
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
	resp, err := e.Client.Get(ctx, path.Join(e.Prefix, clusterKey),
		clientv3.WithPrefix(),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %w", err)
	}
	clusters := &corev1.ClusterList{
		Items: []*corev1.Cluster{},
	}
	selectorPredicate := storage.NewSelectorPredicate[*corev1.Cluster](&corev1.ClusterSelector{
		LabelSelector: matchLabels,
		MatchOptions:  matchOptions,
	})

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

	retryFunc := func() error {
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
				logger.Err(err),
			).Error("error updating cluster")
			return err
		}
		if !txnResp.Succeeded {
			return errRetry
		}
		cluster.SetResourceVersion(fmt.Sprint(txnResp.Header.Revision))
		retCluster = cluster
		return nil
	}
	c := defaultBackoff.Start(ctx)
	var err error
	for backoff.Continue(c) {
		err = retryFunc()
		if isRetryErr(err) {
			continue
		}
		break
	}
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
		clientv3.WithRev(version+1),
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
						logger.Err(event.Err()),
					).Error("error watching cluster")
					return
				}
				for _, ev := range event.Events {
					var eventType storage.WatchEventType
					switch ev.Type {
					case mvccpb.DELETE:
						eventType = storage.WatchEventDelete
					case mvccpb.PUT:
						eventType = storage.WatchEventPut
					default:
						continue
					}

					event := storage.WatchEvent[*corev1.Cluster]{
						EventType: eventType,
					}

					if ev.Kv != nil && ev.Kv.Value != nil {
						current := &corev1.Cluster{}
						if err := protojson.Unmarshal(ev.Kv.Value, current); err != nil {
							e.Logger.With(
								logger.Err(err),
							).Error("error unmarshaling cluster")
							continue
						}
						current.SetResourceVersion(fmt.Sprint(ev.Kv.ModRevision))
						event.Current = current
					}
					if ev.PrevKv != nil && ev.PrevKv.Value != nil {
						previous := &corev1.Cluster{}
						if err := protojson.Unmarshal(ev.PrevKv.Value, previous); err != nil {
							e.Logger.With(
								logger.Err(err),
							).Error("error unmarshaling cluster")
							continue
						}
						previous.SetResourceVersion(fmt.Sprint(ev.PrevKv.ModRevision))
						event.Previous = previous
					}

					eventC <- event
				}
			}
		}
	}()
	return eventC, nil
}

func (e *EtcdStore) WatchClusters(
	ctx context.Context,
	knownClusters []*corev1.Cluster,
) (<-chan storage.WatchEvent[*corev1.Cluster], error) {
	resp, err := e.Client.Get(ctx, path.Join(e.Prefix, clusterKey), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %w", err)
	}
	startRev := resp.Header.Revision
	knownClusterMap := make(map[string]*corev1.Cluster, len(knownClusters))
	for _, cluster := range knownClusters {
		knownClusterMap[cluster.Id] = cluster
	}
	initialEvents := make([]storage.WatchEvent[*corev1.Cluster], 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		cluster := &corev1.Cluster{}
		if err := protojson.Unmarshal(kv.Value, cluster); err != nil {
			return nil, fmt.Errorf("failed to unmarshal cluster: %w", err)
		}
		cluster.SetResourceVersion(fmt.Sprint(kv.ModRevision))
		if knownCluster, ok := knownClusterMap[cluster.Id]; !ok {
			// cluster was not known
			initialEvents = append(initialEvents, storage.WatchEvent[*corev1.Cluster]{
				EventType: storage.WatchEventPut,
				Current:   cluster,
			})
		} else if knownCluster.GetResourceVersion() != cluster.GetResourceVersion() {
			// cluster was known, but resource version has changed
			initialEvents = append(initialEvents, storage.WatchEvent[*corev1.Cluster]{
				EventType: storage.WatchEventPut,
				Current:   cluster,
				Previous:  knownCluster,
			})
		}
	}
	bufSize := 100
	for len(initialEvents) > bufSize {
		bufSize *= 2
	}
	eventC := make(chan storage.WatchEvent[*corev1.Cluster], bufSize)

	// send create or update events for unknown clusters
	for _, event := range initialEvents {
		eventC <- event
	}

	wc := e.Client.Watch(ctx, path.Join(e.Prefix, clusterKey),
		clientv3.WithPrefix(),
		clientv3.WithPrevKV(),
		clientv3.WithRev(startRev+1),
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
						logger.Err(event.Err()),
					).Error("error watching clusters")
					return
				}
				for _, ev := range event.Events {
					var eventType storage.WatchEventType
					current := &corev1.Cluster{}
					previous := &corev1.Cluster{}
					if ev.Type == mvccpb.DELETE {
						eventType = storage.WatchEventDelete
					}
					if ev.IsCreate() {
						// created
						if err := protojson.Unmarshal(ev.Kv.Value, current); err != nil {
							e.Logger.With(
								logger.Err(err),
							).Error("error unmarshaling cluster")
							continue
						}
						current.SetResourceVersion(fmt.Sprint(ev.Kv.CreateRevision))
						previous = nil
						eventType = storage.WatchEventPut
					} else {
						if ev.IsModify() {
							if err := protojson.Unmarshal(ev.Kv.Value, current); err != nil {
								e.Logger.With(
									logger.Err(err),
								).Error("error unmarshaling cluster")
								continue
							}
							current.SetResourceVersion(fmt.Sprint(ev.Kv.ModRevision))
							eventType = storage.WatchEventPut
						} else {
							// deleted
							current = nil
						}
						// if we get here, version is > 1 or 0, therefore PrevKv will always be set
						if err := protojson.Unmarshal(ev.PrevKv.Value, previous); err != nil {
							e.Logger.With(
								logger.Err(err),
							).Error("error unmarshaling cluster")
							continue
						}
						previous.SetResourceVersion(fmt.Sprint(ev.PrevKv.ModRevision))
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
