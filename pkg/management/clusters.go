package management

import (
	context "context"
	"errors"

	core "github.com/kralicky/opni-monitoring/pkg/core"
	"github.com/kralicky/opni-monitoring/pkg/storage"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

func (m *Server) ListClusters(
	ctx context.Context,
	in *ListClustersRequest,
) (*core.ClusterList, error) {
	clusters, err := m.clusterStore.ListClusters(ctx, in.MatchLabels)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	clusterList := &core.ClusterList{
		Items: make([]*core.Cluster, len(clusters.Items)),
	}
	for i, cluster := range clusters.Items {
		clusterList.Items[i] = &core.Cluster{
			Id: cluster.Id,
		}
	}
	return clusterList, nil
}

func (m *Server) DeleteCluster(
	ctx context.Context,
	ref *core.Reference,
) error {
	return grpcError(m.clusterStore.DeleteCluster(ctx, ref))
}

func (m *Server) GetCluster(
	ctx context.Context,
	ref *core.Reference,
) (*core.Cluster, error) {
	if cluster, err := m.clusterStore.GetCluster(ctx, ref); err != nil {
		return nil, grpcError(err)
	} else {
		return cluster, nil
	}
}

func (m *Server) EditCluster(
	ctx context.Context,
	in *EditClusterRequest,
) (*core.Cluster, error) {
	storedCluster, err := m.clusterStore.GetCluster(ctx, in.Cluster)
	if err != nil {
		return nil, grpcError(err)
	}
	storedCluster.Labels = in.Labels
	if c, err := m.clusterStore.UpdateCluster(ctx, storedCluster); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	} else {
		return c, nil
	}
}
