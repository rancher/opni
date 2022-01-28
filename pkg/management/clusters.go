package management

import (
	context "context"

	core "github.com/kralicky/opni-monitoring/pkg/core"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

func (m *Server) ListClusters(
	ctx context.Context,
	in *ListClustersRequest,
) (*core.ClusterList, error) {
	clusterList, err := m.clusterStore.ListClusters(ctx, in.MatchLabels)
	if err != nil {
		return nil, grpcError(err)
	}
	return clusterList, nil
}

func (m *Server) DeleteCluster(
	ctx context.Context,
	ref *core.Reference,
) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, grpcError(m.clusterStore.DeleteCluster(ctx, ref))
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
		return nil, grpcError(err)
	} else {
		return c, nil
	}
}
