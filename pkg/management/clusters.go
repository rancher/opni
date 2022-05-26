package management

import (
	"context"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (m *Server) ListClusters(
	ctx context.Context,
	in *managementv1.ListClustersRequest,
) (*corev1.ClusterList, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	clusterList, err := m.coreDataSource.StorageBackend().ListClusters(ctx, in.MatchLabels, in.MatchOptions)
	if err != nil {
		return nil, err
	}
	return clusterList, nil
}

func (m *Server) DeleteCluster(
	ctx context.Context,
	ref *corev1.Reference,
) (*emptypb.Empty, error) {
	if err := validation.Validate(ref); err != nil {
		return nil, err
	}
	cluster, err := m.coreDataSource.StorageBackend().GetCluster(ctx, ref)
	if err != nil {
		return nil, err
	}
	capabilities := cluster.GetMetadata().GetCapabilities()
	var capabilityNames []string
	for _, cap := range capabilities {
		capabilityNames = append(capabilityNames, cap.Name)
	}
	err = m.capabilitiesDataSource.CapabilitiesStore().UninstallCapabilities(ref, capabilityNames...)
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, m.coreDataSource.StorageBackend().DeleteCluster(ctx, ref)
}

func (m *Server) GetCluster(
	ctx context.Context,
	ref *corev1.Reference,
) (*corev1.Cluster, error) {
	if err := validation.Validate(ref); err != nil {
		return nil, err
	}
	if cluster, err := m.coreDataSource.StorageBackend().GetCluster(ctx, ref); err != nil {
		return nil, err
	} else {
		return cluster, nil
	}
}

func (m *Server) WatchClusters(
	in *managementv1.WatchClustersRequest,
	stream managementv1.Management_WatchClustersServer,
) error {
	if err := validation.Validate(in); err != nil {
		return err
	}
	known := map[string]*corev1.Reference{}
	for _, cluster := range in.KnownClusters.Items {
		if _, err := m.coreDataSource.StorageBackend().GetCluster(context.Background(), cluster); err != nil {
			return err
		}
		known[cluster.Id] = cluster
	}
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			clusters, err := m.coreDataSource.StorageBackend().ListClusters(context.Background(), nil, 0)
			updatedIds := map[string]struct{}{}
			if err != nil {
				return err
			}
			for _, cluster := range clusters.Items {
				updatedIds[cluster.Id] = struct{}{}
				if _, ok := known[cluster.Id]; !ok {
					ref := cluster.Reference()
					known[cluster.Id] = ref
					if err := stream.Send(&managementv1.WatchEvent{
						Cluster: ref,
						Type:    managementv1.WatchEventType_Added,
					}); err != nil {
						return status.Error(codes.Internal, err.Error())
					}
				}
			}
			for id, cluster := range known {
				if _, ok := updatedIds[id]; !ok {
					delete(known, id)
					if err := stream.Send(&managementv1.WatchEvent{
						Cluster: cluster,
						Type:    managementv1.WatchEventType_Deleted,
					}); err != nil {
						return status.Error(codes.Internal, err.Error())
					}
				}
			}
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

func (m *Server) EditCluster(
	ctx context.Context,
	in *managementv1.EditClusterRequest,
) (*corev1.Cluster, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	return m.coreDataSource.StorageBackend().UpdateCluster(ctx, in.GetCluster(), func(cluster *corev1.Cluster) {
		if cluster.Metadata == nil {
			cluster.Metadata = &corev1.ClusterMetadata{}
		}
		cluster.Metadata.Labels = in.GetLabels()
	})
}
