package test

import (
	"context"
	"sync"

	"github.com/golang/mock/gomock"
	"github.com/kralicky/opni-monitoring/pkg/core"
	"github.com/kralicky/opni-monitoring/pkg/storage"
	mock_storage "github.com/kralicky/opni-monitoring/pkg/test/mock/storage"
)

func NewTestClusterStore(ctrl *gomock.Controller) storage.ClusterStore {
	mockClusterStore := mock_storage.NewMockClusterStore(ctrl)

	clusters := map[string]*core.Cluster{}
	mu := sync.Mutex{}

	mockClusterStore.EXPECT().
		CreateCluster(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, cluster *core.Cluster) error {
			mu.Lock()
			defer mu.Unlock()
			clusters[cluster.Id] = cluster
			return nil
		}).
		AnyTimes()
	mockClusterStore.EXPECT().
		DeleteCluster(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, ref *core.Reference) error {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := clusters[ref.Id]; !ok {
				return storage.ErrNotFound
			}
			delete(clusters, ref.Id)
			return nil
		}).
		AnyTimes()
	mockClusterStore.EXPECT().
		ClusterExists(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, ref *core.Reference) (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := clusters[ref.Id]; !ok {
				return false, nil
			}
			return true, nil
		}).
		AnyTimes()
	mockClusterStore.EXPECT().
		ListClusters(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, matchLabels *core.LabelSelector) (*core.ClusterList, error) {
			mu.Lock()
			defer mu.Unlock()
			clusterList := &core.ClusterList{}
			selectorPredicate := storage.ClusterSelector{
				LabelSelector: matchLabels,
			}.Predicate()
			for _, cluster := range clusters {
				if !selectorPredicate(cluster) {
					continue
				}
				clusterList.Items = append(clusterList.Items, cluster)
			}
			return clusterList, nil
		}).
		AnyTimes()
	mockClusterStore.EXPECT().
		GetCluster(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, ref *core.Reference) (*core.Cluster, error) {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := clusters[ref.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			return clusters[ref.Id], nil
		}).
		AnyTimes()
	mockClusterStore.EXPECT().
		UpdateCluster(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, cluster *core.Cluster) error {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := clusters[cluster.Id]; !ok {
				return storage.ErrNotFound
			}
			clusters[cluster.Id] = cluster
			return nil
		}).
		AnyTimes()
	return mockClusterStore
}
