package backend

import (
	"context"
	"fmt"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/opensearch/opensearch/types"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/wait"
	"os"
	"time"
)

func (b *LoggingBackend) waitForOpensearchClient(ctx context.Context) error {
	b.WaitForInit()

	var retErr error
	stopChan := make(chan struct{})

	wait.Until(func() {
		select {
		case <-ctx.Done():
			retErr = fmt.Errorf("context cancelled before client was set")
			close(stopChan)
		default:
		}
		if b.OpensearchManager.Client != nil {
			close(stopChan)
		} else {
			b.Logger.Errorf("opensearch client not set, waiting")
		}
	}, time.Second, stopChan)

	return retErr
}

func (b *LoggingBackend) updateClusterMetadata(ctx context.Context, event *managementv1.WatchEvent) error {
	newName, oldName := event.Cluster.Metadata.Labels[opnicorev1.NameLabel], event.PreviousCluster.Metadata.Labels[opnicorev1.NameLabel]
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

	updateDoc := types.ClusterMetadataDocUpdate{
		Name: newName,
	}

	if err := b.OpensearchManager.UpdateClusterMetadata(ctx, event.Cluster.Reference(), updateDoc); err != nil {
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
		case <-ctx.Done():
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
