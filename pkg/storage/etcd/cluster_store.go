package etcd

import (
	"context"
	"fmt"
	"path"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/storage"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
	"k8s.io/client-go/util/retry"
)

func (e *EtcdStore) CreateCluster(ctx context.Context, cluster *core.Cluster) error {
	data, err := protojson.Marshal(cluster)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster: %w", err)
	}
	ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
	defer ca()
	_, err = e.Client.Put(ctx, path.Join(e.Prefix, clusterKey, cluster.Id), string(data))
	if err != nil {
		return fmt.Errorf("failed to create cluster: %w", err)
	}
	return nil
}

func (e *EtcdStore) DeleteCluster(ctx context.Context, ref *core.Reference) error {
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
	matchLabels *core.LabelSelector,
	matchOptions core.MatchOptions,
) (*core.ClusterList, error) {
	ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
	defer ca()
	resp, err := e.Client.Get(ctx, path.Join(e.Prefix, clusterKey),
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
	ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
	defer ca()
	c, _, err := e.getCluster(ctx, ref)
	return c, err
}

func (e *EtcdStore) getCluster(ctx context.Context, ref *core.Reference) (*core.Cluster, int64, error) {
	ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
	defer ca()
	resp, err := e.Client.Get(ctx, path.Join(e.Prefix, clusterKey, ref.Id))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get cluster: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, 0, storage.ErrNotFound
	}
	cluster := &core.Cluster{}
	if err := protojson.Unmarshal(resp.Kvs[0].Value, cluster); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal cluster: %w", err)
	}
	return cluster, resp.Kvs[0].Version, nil
}

func (e *EtcdStore) UpdateCluster(
	ctx context.Context,
	ref *core.Reference,
	mutator storage.MutatorFunc[*core.Cluster],
) (*core.Cluster, error) {
	var retCluster *core.Cluster
	err := retry.OnError(defaultBackoff, isRetryErr, func() error {
		ctx, ca := context.WithTimeout(ctx, e.CommandTimeout)
		defer ca()
		txn := e.Client.Txn(ctx)
		key := path.Join(e.Prefix, clusterKey, ref.Id)
		cluster, version, err := e.getCluster(ctx, ref)
		if err != nil {
			return fmt.Errorf("failed to get cluster: %w", err)
		}
		mutator(cluster)
		data, err := protojson.Marshal(cluster)
		if err != nil {
			return fmt.Errorf("failed to marshal cluster: %w", err)
		}
		txnResp, err := txn.If(clientv3.Compare(clientv3.Version(key), "=", version)).
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
