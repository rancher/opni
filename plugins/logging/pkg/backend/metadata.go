package backend

import (
	"context"
	"os"

	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"go.uber.org/zap"
)

func (b *LoggingBackend) updateClusterMetadata(ctx context.Context, event *managementv1.WatchEvent) error {
	newName, oldName := event.Cluster.Metadata.Labels[opnicorev1.NameLabel], event.Previous.Metadata.Labels[opnicorev1.NameLabel]
	if newName == oldName {
		b.Logger.With(
			"oldName", oldName,
			"newName", newName,
		).Debug("cluster was not renamed")
		return nil
	}

	b.Logger.With(
		"oldName", oldName,
		"newName", newName,
	).Debug("cluster was renamed")

	if err := b.ClusterDriver.StoreClusterMetadata(ctx, event.Cluster.GetId(), newName); err != nil {
		b.Logger.With(
			zap.Error(err),
			"cluster", event.Cluster.Id,
		).Debug("could not update cluster metadata")
		return nil
	}

	return nil
}

func (b *LoggingBackend) watchClusterEvents(ctx context.Context) {
	clusterClient, err := b.MgmtClient.WatchClusters(ctx, &managementv1.WatchClustersRequest{})
	if err != nil {
		b.Logger.With(zap.Error(err)).Errorf("failed to watch clusters, existing")
		os.Exit(1)
	}

	b.Logger.Infof("watching cluster events")

outer:
	for {
		select {
		case <-clusterClient.Context().Done():
			b.Logger.Infof("context cancelled, stoping cluster event watcher")
			break outer
		default:
			event, err := clusterClient.Recv()
			if err != nil {
				b.Logger.With(zap.Error(err)).Errorf("failed to receive cluster event")
				continue
			}

			b.watcher.HandleEvent(event)
		}
	}
}

func (b *LoggingBackend) reconcileClusterMetadata(ctx context.Context, clusters []*opnicorev1.Cluster) (retErr error) {
	for _, cluster := range clusters {
		err := b.ClusterDriver.StoreClusterMetadata(ctx, cluster.GetId(), cluster.Metadata.Labels[opnicorev1.NameLabel])
		if err != nil {
			b.Logger.With(
				zap.Error(err),
				"cluster", cluster.Id,
			).Warn("could not update cluster metadata")
			retErr = err
		}
	}
	return
}
