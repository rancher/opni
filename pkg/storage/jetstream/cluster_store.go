package jetstream

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/nats-io/nats.go"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"google.golang.org/protobuf/encoding/protojson"
)

func (s *JetStreamStore) CreateCluster(ctx context.Context, cluster *corev1.Cluster) error {
	cluster.SetResourceVersion("")

	data, err := protojson.Marshal(cluster)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster: %w", err)
	}
	rev, err := s.kv.Clusters.Create(cluster.Id, data)
	if err != nil {
		return fmt.Errorf("failed to create cluster: %w", err)
	}
	cluster.SetResourceVersion(fmt.Sprint(rev))
	return nil
}

func (s *JetStreamStore) DeleteCluster(ctx context.Context, ref *corev1.Reference) error {
	_, err := s.GetCluster(ctx, ref)
	if err != nil {
		return err
	}
	return s.kv.Clusters.Delete(ref.Id)
}

func (s *JetStreamStore) GetCluster(ctx context.Context, ref *corev1.Reference) (*corev1.Cluster, error) {
	resp, err := s.kv.Clusters.Get(ref.Id)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return nil, storage.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get cluster: %w", err)
	}
	cluster := &corev1.Cluster{}
	if err := protojson.Unmarshal(resp.Value(), cluster); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster: %w", err)
	}
	cluster.SetResourceVersion(fmt.Sprint(resp.Revision()))
	return cluster, nil
}

func (s *JetStreamStore) UpdateCluster(ctx context.Context, ref *corev1.Reference, mutator storage.ClusterMutator) (*corev1.Cluster, error) {
	p := backoff.Exponential(
		backoff.WithMaxRetries(0),
		backoff.WithMinInterval(1*time.Millisecond),
		backoff.WithMaxInterval(128*time.Millisecond),
		backoff.WithMultiplier(2),
	)
	b := p.Start(ctx)
	var updateErr error
	for backoff.Continue(b) {
		cluster, err := s.GetCluster(ctx, ref)
		if err != nil {
			return nil, err
		}
		versionStr := cluster.GetResourceVersion()
		version, err := strconv.ParseUint(versionStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("internal error: cluster has invalid resource version: %w", err)
		}
		mutator(cluster)
		cluster.SetResourceVersion("")
		data, err := protojson.Marshal(cluster)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal cluster: %w", err)
		}
		rev, err := s.kv.Clusters.Update(ref.Id, data, version)
		if err != nil {
			updateErr = err
			continue
		}
		cluster.SetResourceVersion(fmt.Sprint(rev))
		return cluster, nil
	}
	if updateErr != nil {
		return nil, fmt.Errorf("failed to update cluster: %w", updateErr)
	}
	return nil, fmt.Errorf("failed to update cluster: (unknown error)")
}

func (s *JetStreamStore) WatchCluster(ctx context.Context, cluster *corev1.Cluster) (<-chan storage.WatchEvent[*corev1.Cluster], error) {
	eventC := make(chan storage.WatchEvent[*corev1.Cluster], 10)

	w, err := s.kv.Clusters.Watch(cluster.Id, nats.Context(ctx))
	if err != nil {
		return nil, err
	}

	go func() {
		defer w.Stop()
		updates := w.Updates()
		current := cluster
	INITIAL:
		for {
			select {
			case <-ctx.Done():
				return
			case update := <-updates:
				if update == nil {
					break INITIAL
				}
				if e, ok := s.translateClusterWatchEvent(update); ok {
					if e.Current.GetResourceVersion() != cluster.GetResourceVersion() {
						// only send the initial update if the resource version has changed
						eventC <- storage.WatchEvent[*corev1.Cluster]{
							EventType: storage.WatchEventUpdate,
							Current:   e.Current,
							Previous:  cluster,
						}
						current = e.Current
					}
				}
			}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case update := <-updates:
				if update == nil {
					return
				}
				if e, ok := s.translateClusterWatchEvent(update); ok {
					if e.EventType == storage.WatchEventUpdate {
						e.Previous = current
						current = e.Current
					} else if e.EventType == storage.WatchEventDelete {
						e.Previous = current
						current = nil
					}
					eventC <- e
				}
			}
		}
	}()

	return eventC, nil
}

func (s *JetStreamStore) translateClusterWatchEvent(update nats.KeyValueEntry) (storage.WatchEvent[*corev1.Cluster], bool) {
	switch update.Operation() {
	case nats.KeyValuePut:
		cluster := &corev1.Cluster{}
		if err := protojson.Unmarshal(update.Value(), cluster); err != nil {
			s.logger.With(
				"cluster", update.Key(),
				"error", err,
			).Warn("failed to unmarshal cluster")
			return storage.WatchEvent[*corev1.Cluster]{}, false
		}
		cluster.SetResourceVersion(fmt.Sprint(update.Revision()))
		return storage.WatchEvent[*corev1.Cluster]{
			EventType: storage.WatchEventUpdate,
			Current:   cluster,
		}, true
	case nats.KeyValueDelete, nats.KeyValuePurge:
		return storage.WatchEvent[*corev1.Cluster]{
			EventType: storage.WatchEventDelete,
			Previous:  &corev1.Cluster{Id: update.Key()},
		}, true
	}

	panic("bug: unknown kv operation: " + update.Operation().String())
}

func (s *JetStreamStore) WatchClusters(ctx context.Context, knownClusters []*corev1.Cluster) (<-chan storage.WatchEvent[*corev1.Cluster], error) {
	w, err := s.kv.Clusters.WatchAll(nats.Context(ctx))
	if err != nil {
		return nil, err
	}

	var initialEvents []storage.WatchEvent[*corev1.Cluster]
	knownClusterMap := make(map[string]*corev1.Cluster, len(knownClusters))
	for _, cluster := range knownClusters {
		knownClusterMap[cluster.Id] = cluster
	}

	updates := w.Updates()
INITIAL:
	for {
		select {
		case <-ctx.Done():
			w.Stop()
			return nil, ctx.Err()
		case update := <-updates:
			if update == nil {
				break INITIAL
			}
			if update.Operation() == nats.KeyValueDelete {
				continue
			}
			if e, ok := s.translateClusterWatchEvent(update); ok {
				if knownCluster, ok := knownClusterMap[e.Current.Id]; !ok {
					// cluster was not known
					initialEvents = append(initialEvents, storage.WatchEvent[*corev1.Cluster]{
						EventType: storage.WatchEventCreate,
						Current:   e.Current,
					})
					knownClusterMap[e.Current.Id] = e.Current
				} else if knownCluster.GetResourceVersion() != e.Current.GetResourceVersion() {
					// cluster was known, but resource version has changed
					initialEvents = append(initialEvents, storage.WatchEvent[*corev1.Cluster]{
						EventType: storage.WatchEventUpdate,
						Current:   e.Current,
						Previous:  knownCluster,
					})
					knownClusterMap[e.Current.Id] = e.Current
				}
			}
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

	go func() {
		defer w.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case update := <-updates:
				if update == nil {
					return
				}
				if e, ok := s.translateClusterWatchEvent(update); ok {
					switch e.EventType {
					case storage.WatchEventUpdate:
						if prev, ok := knownClusterMap[e.Current.Id]; !ok {
							e.EventType = storage.WatchEventCreate
						} else {
							e.Previous = prev
						}
						knownClusterMap[e.Current.Id] = e.Current
					case storage.WatchEventDelete:
						e.Previous = knownClusterMap[e.Previous.Id]
						delete(knownClusterMap, e.Previous.Id)
					}
					eventC <- e
				}
			}
		}
	}()

	return eventC, nil
}

func (s *JetStreamStore) ListClusters(ctx context.Context, matchLabels *corev1.LabelSelector, matchOptions corev1.MatchOptions) (*corev1.ClusterList, error) {
	watcher, err := s.kv.Clusters.WatchAll(nats.IgnoreDeletes(), nats.Context(ctx))
	if err != nil {
		return nil, err
	}
	defer watcher.Stop()

	selectorPredicate := storage.NewSelectorPredicate(&corev1.ClusterSelector{
		LabelSelector: matchLabels,
		MatchOptions:  matchOptions,
	})

	var clusters []*corev1.Cluster
	for entry := range watcher.Updates() {
		if entry == nil {
			break
		}

		cluster := &corev1.Cluster{}
		if err := protojson.Unmarshal(entry.Value(), cluster); err != nil {
			return nil, err
		}
		if selectorPredicate(cluster) {
			cluster.SetResourceVersion(fmt.Sprint(entry.Revision()))
			clusters = append(clusters, cluster)
		}
	}
	return &corev1.ClusterList{
		Items: clusters,
	}, nil
}
