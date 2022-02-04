package etcd

import (
	"context"
	"fmt"
	"path"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/storage"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/encoding/protojson"
)

func (e *EtcdStore) CreateCluster(ctx context.Context, cluster *core.Cluster) error {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	data, err := protojson.Marshal(cluster)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster: %w", err)
	}
	_, err = e.client.Put(ctx, path.Join(e.namespace, clusterKey, cluster.Id), string(data))
	if err != nil {
		return fmt.Errorf("failed to create cluster: %w", err)
	}
	return nil
}

func (e *EtcdStore) DeleteCluster(ctx context.Context, ref *core.Reference) error {
	if err := ref.CheckValidID(); err != nil {
		return err
	}
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	_, err := e.client.Delete(ctx, path.Join(e.namespace, clusterKey, ref.Id), clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("failed to delete cluster: %w", err)
	}
	return nil
}

func (e *EtcdStore) ClusterExists(ctx context.Context, ref *core.Reference) (bool, error) {
	if err := ref.CheckValidID(); err != nil {
		return false, err
	}
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Get(ctx, path.Join(e.namespace, clusterKey, ref.Id))
	if err != nil {
		return false, fmt.Errorf("failed to get cluster: %w", err)
	}
	return len(resp.Kvs) > 0, nil
}

func (e *EtcdStore) ListClusters(
	ctx context.Context,
	matchLabels *core.LabelSelector,
	matchOptions core.MatchOptions,
) (*core.ClusterList, error) {
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Get(ctx, path.Join(e.namespace, clusterKey),
		clientv3.WithPrefix(),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %w", err)
	}
	clusters := &core.ClusterList{
		Items: []*core.Cluster{},
	}
	selectorPredicate := storage.ClusterSelector{
		LabelSelector: matchLabels,
		MatchOptions:  matchOptions,
	}.Predicate()

	for _, kv := range resp.Kvs {
		cluster := &core.Cluster{}
		if err := protojson.Unmarshal(kv.Value, cluster); err != nil {
			return nil, fmt.Errorf("failed to unmarshal cluster: %w", err)
		}
		if selectorPredicate(cluster) {
			clusters.Items = append(clusters.Items, cluster)
		}
	}
	return clusters, nil
}

func (e *EtcdStore) GetCluster(ctx context.Context, ref *core.Reference) (*core.Cluster, error) {
	if err := ref.CheckValidID(); err != nil {
		return nil, err
	}
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	resp, err := e.client.Get(ctx, path.Join(e.namespace, clusterKey, ref.Id))
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("failed to get cluster: %w", storage.ErrNotFound)
	}
	cluster := &core.Cluster{}
	if err := protojson.Unmarshal(resp.Kvs[0].Value, cluster); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cluster: %w", err)
	}
	return cluster, nil
}

func (e *EtcdStore) UpdateCluster(
	ctx context.Context,
	cluster *core.Cluster,
) (*core.Cluster, error) {
	if exists, err := e.ClusterExists(ctx, cluster.Reference()); err != nil {
		return nil, err
	} else if !exists {
		return nil, fmt.Errorf("failed to update cluster: %w", storage.ErrNotFound)
	}
	ctx, ca := context.WithTimeout(ctx, defaultEtcdTimeout)
	defer ca()
	data, err := protojson.Marshal(cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal cluster: %w", err)
	}
	_, err = e.client.Put(ctx, path.Join(e.namespace, clusterKey, cluster.Id), string(data))
	if err != nil {
		return nil, fmt.Errorf("failed to update cluster: %w", err)
	}
	return cluster, nil
}
