package management

import (
	"context"
	"fmt"
	"maps"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/validation"
	"github.com/samber/lo"
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
	cluster, err := m.resolveCluster(ctx, ref)
	if err != nil {
		return nil, err
	}
	capabilities := cluster.GetMetadata().GetCapabilities()
	if len(capabilities) > 0 {
		return nil, status.Error(codes.FailedPrecondition, "cannot delete a cluster with capabilities; uninstall the capabilities first")
	}
	// delete the cluster's keyring, if it exists
	store := m.coreDataSource.StorageBackend().KeyringStore("gateway", cluster.Reference())
	if err := store.Delete(ctx); err != nil {
		if !storage.IsNotFound(err) {
			return nil, fmt.Errorf("failed to delete keyring store for cluster %s: %w", cluster.Id, err)
		}
	}

	// delete the cluster
	err = m.coreDataSource.StorageBackend().DeleteCluster(ctx, cluster.Reference())
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
	return m.resolveCluster(ctx, ref)
}

func (m *Server) WatchClusters(
	in *managementv1.WatchClustersRequest,
	stream managementv1.Management_WatchClustersServer,
) error {
	if err := validation.Validate(in); err != nil {
		return err
	}
	known, err := m.resolveClusters(stream.Context(), in.GetKnownClusters().GetItems()...)
	if err != nil {
		return err
	}

	eventC, err := m.coreDataSource.StorageBackend().WatchClusters(stream.Context(), known)
	if err != nil {
		return err
	}

	for event := range eventC {
		var c, o *corev1.Cluster
		var eventType managementv1.WatchEventType
		switch event.EventType {
		case storage.WatchEventPut:
			eventType = managementv1.WatchEventType_Put
			c = event.Current
			o = event.Previous
		case storage.WatchEventDelete:
			eventType = managementv1.WatchEventType_Delete
			c = event.Previous
			o = event.Previous
		}
		if err := stream.Send(&managementv1.WatchEvent{
			Cluster:  c,
			Type:     eventType,
			Previous: o,
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

	oldCluster, err := m.resolveCluster(ctx, in.GetCluster())
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

	return m.coreDataSource.StorageBackend().UpdateCluster(ctx, oldCluster.Reference(), func(cluster *corev1.Cluster) {
		if cluster.Metadata == nil {
			cluster.Metadata = &corev1.ClusterMetadata{}
		}

		cluster.Metadata.Labels = in.GetLabels()
	})
}

func (m *Server) InstallCapability(
	ctx context.Context,
	in *capabilityv1.InstallRequest,
) (*capabilityv1.InstallResponse, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	if err := m.ensureReferenceResolved(ctx, in.Agent); err != nil {
		return nil, err
	}

	return m.capabilitiesDataSource.Install(ctx, in)
}

func (m *Server) UninstallCapability(
	ctx context.Context,
	in *capabilityv1.UninstallRequest,
) (*emptypb.Empty, error) {
	if err := validation.Validate(in); err != nil {
		return nil, err
	}
	if err := m.ensureReferenceResolved(ctx, in.Agent); err != nil {
		return nil, err
	}

	return m.capabilitiesDataSource.Uninstall(ctx, in)
}

func (m *Server) CapabilityStatus(
	ctx context.Context,
	req *capabilityv1.StatusRequest,
) (*capabilityv1.NodeCapabilityStatus, error) {
	if err := validation.Validate(req); err != nil {
		return nil, err
	}

	if err := m.ensureReferenceResolved(ctx, req.Agent); err != nil {
		return nil, err
	}

	return m.capabilitiesDataSource.Status(ctx, req)
}

func (m *Server) CapabilityUninstallStatus(
	ctx context.Context,
	req *capabilityv1.UninstallStatusRequest,
) (*corev1.TaskStatus, error) {
	if err := validation.Validate(req); err != nil {
		return nil, err
	}
	if err := m.ensureReferenceResolved(ctx, req.Agent); err != nil {
		return nil, err
	}

	return m.capabilitiesDataSource.UninstallStatus(ctx, req)
}

func (m *Server) CancelCapabilityUninstall(
	ctx context.Context,
	req *capabilityv1.CancelUninstallRequest,
) (*emptypb.Empty, error) {
	if err := validation.Validate(req); err != nil {
		return nil, err
	}
	if err := m.ensureReferenceResolved(ctx, req.Agent); err != nil {
		return nil, err
	}

	return m.capabilitiesDataSource.CancelUninstall(ctx, req)
}

func (m *Server) resolveClusters(ctx context.Context, refs ...*corev1.Reference) ([]*corev1.Cluster, error) {
	clusters := make([]*corev1.Cluster, len(refs))

	for i, ref := range refs {
		cluster, err := m.coreDataSource.StorageBackend().GetCluster(ctx, ref)
		if err != nil {
			if storage.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		clusters[i] = cluster
	}

	var names []string
	nameIndexes := map[string]int{}
	for i, r := range clusters {
		if r == nil {
			names = append(names, refs[i].Id)
			nameIndexes[refs[i].Id] = i
		}
	}

	if len(names) == 0 {
		return clusters, nil
	}

	cl, err := m.coreDataSource.StorageBackend().ListClusters(ctx, &corev1.LabelSelector{
		MatchExpressions: []*corev1.LabelSelectorRequirement{
			{
				Key:      corev1.NameLabel,
				Operator: string(corev1.LabelSelectorOpIn),
				Values:   names,
			},
		},
	}, corev1.MatchOptions_EmptySelectorMatchesNone)
	if err != nil {
		return nil, err
	}

	for _, c := range cl.Items {
		name := c.Metadata.Labels[corev1.NameLabel]
		if idx, ok := nameIndexes[name]; ok {
			clusters[idx] = c
			nameIndexes[name] = -1
		} else if idx == -1 {
			// duplicate
			return nil, status.Errorf(codes.FailedPrecondition, "ambiguous cluster name %q (use cluster IDs instead)", name)
		} else {
			panic("bug: ListClusters with label selector returned bad results")
		}
	}

	for _, idx := range nameIndexes {
		if idx == -1 {
			continue
		}
		notFound := lo.OmitByValues(nameIndexes, []int{-1})
		if len(notFound) > 0 {
			if len(notFound) == 1 {
				return nil, status.Errorf(codes.NotFound, "cluster not found: %q", names[0])
			}
			return nil, status.Errorf(codes.NotFound, "%d clusters not found: %v", len(names), names)
		}
	}

	return clusters, nil
}

func (m *Server) resolveCluster(ctx context.Context, ref *corev1.Reference) (*corev1.Cluster, error) {
	clusters, err := m.resolveClusters(ctx, ref)
	if err != nil {
		return nil, err
	}
	return clusters[0], nil
}

func (m *Server) ensureReferenceResolved(ctx context.Context, ref *corev1.Reference) error {
	c, err := m.resolveCluster(ctx, ref)
	if err != nil {
		return err
	}
	ref.Id = c.Id
	return nil
}
