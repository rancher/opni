package backend

import (
	"context"
	"os"

	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
)

func (b *LoggingBackend) updateClusterMetadata(ctx context.Context, event *managementv1.WatchEvent) error {
	lg := logger.PluginLoggerFromContext(b.Context)
	incomingLabels := event.GetCluster().GetMetadata().GetLabels()
	previousLabels := event.GetPrevious().GetMetadata().GetLabels()
	var newName, oldName string
	if _, ok := incomingLabels[opnicorev1.NameLabel]; ok {
		newName = incomingLabels[opnicorev1.NameLabel]
	}
	if _, ok := previousLabels[opnicorev1.NameLabel]; ok {
		oldName = previousLabels[opnicorev1.NameLabel]
	}
	if newName == oldName {
		lg.With(
			"oldName", oldName,
			"newName", newName,
		).Debug("cluster was not renamed")
		return nil
	}
	lg.With(
		"oldName", oldName,
		"newName", newName,
	).Debug("cluster was renamed")

	if err := b.ClusterDriver.StoreClusterMetadata(ctx, event.Cluster.GetId(), newName); err != nil {
		lg.With(
			logger.Err(err),
			"cluster", event.Cluster.Id,
		).Debug("could not update cluster metadata")
		return nil
	}

	return nil
}

func (b *LoggingBackend) watchClusterEvents(ctx context.Context) {
	lg := logger.PluginLoggerFromContext(b.Context)

	clusterClient, err := b.MgmtClient.WatchClusters(ctx, &managementv1.WatchClustersRequest{})
	if err != nil {
		lg.With(logger.Err(err)).Error("failed to watch clusters, existing")
		os.Exit(1)
	}

	lg.Info("watching cluster events")

outer:
	for {
		select {
		case <-clusterClient.Context().Done():
			lg.Info("context cancelled, stoping cluster event watcher")
			break outer
		default:
			event, err := clusterClient.Recv()
			if err != nil {
				lg.With(logger.Err(err)).Error("failed to receive cluster event")
				continue
			}

			b.watcher.HandleEvent(event)
		}
	}
}

func (b *LoggingBackend) reconcileClusterMetadata(ctx context.Context, clusters []*opnicorev1.Cluster) (retErr error) {
	lg := logger.PluginLoggerFromContext(b.Context)

	for _, cluster := range clusters {
		err := b.ClusterDriver.StoreClusterMetadata(ctx, cluster.GetId(), cluster.Metadata.Labels[opnicorev1.NameLabel])
		if err != nil {
			lg.With(
				logger.Err(err),
				"cluster", cluster.Id,
			).Warn("could not update cluster metadata")
			retErr = err
		}
	}
	return
}
