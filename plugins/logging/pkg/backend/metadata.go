package backend

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/opensearch/opensearch/types"
	"go.uber.org/zap"
)

func (b *LoggingBackend) waitForOpensearchClient(ctx context.Context) error {
	b.WaitForInit()

	expBackoff := backoff.Exponential(
		backoff.WithMaxRetries(0),
		backoff.WithMinInterval(5*time.Second),
		backoff.WithMaxInterval(1*time.Minute),
		backoff.WithMultiplier(1.1),
	).Start(ctx)

CHECK:
	for {
		select {
		case <-expBackoff.Done():
			return fmt.Errorf("context cancelled before client was set")
		case <-expBackoff.Next():
			b.OpensearchManager.Lock()
			client := b.OpensearchManager.Client
			b.OpensearchManager.Unlock()
			if client != nil {
				break CHECK
			} else {
				b.Logger.Errorf("opensearch client not set, waiting")
			}
		}
	}

	return nil
}

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
