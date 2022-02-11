package test

import (
	"context"
	"sync"

	"github.com/golang/mock/gomock"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/storage"
	mock_storage "github.com/rancher/opni-monitoring/pkg/test/mock/storage"
	"google.golang.org/protobuf/proto"
)

func NewTestClusterStore(ctrl *gomock.Controller) storage.ClusterStore {
	mockClusterStore := mock_storage.NewMockClusterStore(ctrl)

	clusters := map[string]*core.Cluster{}
	mu := sync.Mutex{}

	mockClusterStore.EXPECT().
		CreateCluster(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, cluster *core.Cluster) error {
			mu.Lock()
			defer mu.Unlock()
			clusters[cluster.Id] = cluster
			return nil
		}).
		AnyTimes()
	mockClusterStore.EXPECT().
		DeleteCluster(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *core.Reference) error {
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
		ListClusters(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, matchLabels *core.LabelSelector, matchOptions core.MatchOptions) (*core.ClusterList, error) {
			mu.Lock()
			defer mu.Unlock()
			clusterList := &core.ClusterList{}
			selectorPredicate := storage.ClusterSelector{
				LabelSelector: matchLabels,
				MatchOptions:  matchOptions,
			}.Predicate()
			for _, cluster := range clusters {
				if selectorPredicate(cluster) {
					clusterList.Items = append(clusterList.Items, cluster)
				}
			}
			return clusterList, nil
		}).
		AnyTimes()
	mockClusterStore.EXPECT().
		GetCluster(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, ref *core.Reference) (*core.Cluster, error) {
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
		DoAndReturn(func(_ context.Context, cluster *core.Cluster) (*core.Cluster, error) {
			mu.Lock()
			defer mu.Unlock()
			cloned := proto.Clone(cluster).(*core.Cluster)
			if _, ok := clusters[cloned.Id]; !ok {
				return nil, storage.ErrNotFound
			}
			clusters[cloned.Id] = cloned
			return cloned, nil
		}).
		AnyTimes()
	return mockClusterStore
}
