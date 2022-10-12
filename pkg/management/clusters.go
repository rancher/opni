package management

import (
	"context"
	"errors"
	"fmt"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/validation"
	"github.com/samber/lo"
	"golang.org/x/exp/maps"
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
	if len(capabilities) > 0 {
		return nil, status.Error(codes.FailedPrecondition, "cannot delete a cluster with capabilities; uninstall the capabilities first")
	}
	// delete the cluster's keyring, if it exists
	store := m.coreDataSource.StorageBackend().KeyringStore("gateway", ref)
	if err := store.Delete(ctx); err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("failed to delete keyring store for cluster %s: %w", ref.Id, err)
		}
	}

	// delete the cluster
	err = m.coreDataSource.StorageBackend().DeleteCluster(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
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
	var known []*corev1.Cluster
	for _, cluster := range in.GetKnownClusters().GetItems() {
		if c, err := m.coreDataSource.StorageBackend().GetCluster(stream.Context(), cluster); err != nil {
			return err
		} else {
			known = append(known, c)
		}
	}

	eventC, err := m.coreDataSource.StorageBackend().WatchClusters(stream.Context(), known)
	if err != nil {
		return err
	}

	for event := range eventC {
		var c *corev1.Cluster
		var eventType managementv1.WatchEventType
		switch event.EventType {
		case storage.WatchEventCreate:
			eventType = managementv1.WatchEventType_Created
			c = event.Current
		case storage.WatchEventUpdate:
			eventType = managementv1.WatchEventType_Updated
			c = event.Current
		case storage.WatchEventDelete:
			eventType = managementv1.WatchEventType_Deleted
			c = event.Previous
		}
		if err := stream.Send(&managementv1.WatchEvent{
			Cluster: c,
			Type:    eventType,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Server) EditCluster(
	ctx context.Context,
	in *managementv1.EditClusterRequest,
) (*corev1.Cluster, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}

	oldCluster, err := m.coreDataSource.StorageBackend().GetCluster(ctx, in.GetCluster())
	if err != nil {
		return nil, err
	}

	oldLabels := oldCluster.GetMetadata().GetLabels()

	// ensure immutable labels are not modified
	oldImmutableLabels := lo.PickBy(oldLabels, func(k string, _ string) bool {
		return !corev1.IsLabelMutable(k)
	})
	newImmutableLabels := lo.PickBy(in.GetLabels(), func(k string, _ string) bool {
		return !corev1.IsLabelMutable(k)
	})
	if !maps.Equal(oldImmutableLabels, newImmutableLabels) {
		return nil, status.Error(codes.InvalidArgument, "cannot change immutable labels")
	}

	return m.coreDataSource.StorageBackend().UpdateCluster(ctx, in.GetCluster(), func(cluster *corev1.Cluster) {
		if cluster.Metadata == nil {
			cluster.Metadata = &corev1.ClusterMetadata{}
		}

		cluster.Metadata.Labels = in.GetLabels()
	})
}

func (m *Server) InstallCapability(
	ctx context.Context,
	in *managementv1.CapabilityInstallRequest,
) (*capabilityv1.InstallResponse, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}

	backendStore := m.capabilitiesDataSource.CapabilitiesStore()
	backend, err := backendStore.Get(in.Name)
	if err != nil {
		return nil, err
	}

	resp, err := backend.Install(ctx, in.Target)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return &capabilityv1.InstallResponse{
			Status: capabilityv1.InstallResponseStatus_Success,
		}, nil
	}
	return resp, nil
}

func (m *Server) UninstallCapability(
	ctx context.Context,
	in *managementv1.CapabilityUninstallRequest,
) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}

	backendStore := m.capabilitiesDataSource.CapabilitiesStore()
	backend, err := backendStore.Get(in.Name)
	if err != nil {
		return nil, err
	}

	_, err = backend.Uninstall(ctx, in.Target)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (m *Server) CapabilityUninstallStatus(
	ctx context.Context,
	req *managementv1.CapabilityStatusRequest,
) (*corev1.TaskStatus, error) {
	if err := validation.Validate(req); err != nil {
		return nil, err
	}

	backendStore := m.capabilitiesDataSource.CapabilitiesStore()
	backend, err := backendStore.Get(req.Name)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, status.Errorf(codes.NotFound, "capability not found for cluster %s", req.Cluster.Id)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	stat, err := backend.UninstallStatus(ctx, req.Cluster)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, status.Errorf(codes.NotFound, "no status available for cluster %s and capability %s", req.Cluster, req.Name)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return stat, nil
}

func (m *Server) CancelCapabilityUninstall(
	ctx context.Context,
	req *managementv1.CapabilityUninstallCancelRequest,
) (*emptypb.Empty, error) {
	if err := validation.Validate(req); err != nil {
		return nil, err
	}

	cluster, err := m.coreDataSource.StorageBackend().GetCluster(ctx, req.Cluster)
	if err != nil {
		return nil, err
	}

	backendStore := m.capabilitiesDataSource.CapabilitiesStore()
	backend, err := backendStore.Get(req.Name)
	if err != nil {
		return nil, err
	}

	_, err = backend.CancelUninstall(ctx, cluster.Reference())
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
