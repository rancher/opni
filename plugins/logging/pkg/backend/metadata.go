package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/opensearch-project/opensearch-go/v2/opensearchutil"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/opensearch/opensearch/types"
	"github.com/rancher/opni/pkg/resources/multiclusterrolebinding"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"io"
	"k8s.io/apimachinery/pkg/util/wait"
	"net/http"
	"os"
	"time"
)

func (b *LoggingBackend) waitForOpensearchClient(ctx context.Context) error {
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

func (b *LoggingBackend) reconcileClusterDoc(ctx context.Context, doc *types.ClusterMetadataDoc, cluster *opnicorev1.Cluster) error {
	if currentName := cluster.Metadata.Labels[opnicorev1.NameLabel]; doc.Name != currentName {
		b.Logger.With(
			"cluster", cluster.Id,
		).Debug("cluster name has changed")

		resp, err := b.OpensearchManager.Indices.UpdateDocument(ctx, multiclusterrolebinding.ClusterMetadataIndexName, cluster.Id, opensearchutil.NewJSONReader(
			types.MetadataUpdate{
				Document: types.ClusterMetadataDocUpdate{
					Name: currentName,
				},
			},
		))
		if err != nil {
			return fmt.Errorf("could not update cluster: %w", err)
		}
		defer resp.Body.Close()

		if resp.IsError() {
			errMsg, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read response body: %w", err)
			}

			return fmt.Errorf("failed to update cluster to opensearch cluster metadata index: %s", errMsg)
		}
	}

	return nil
}

func (b *LoggingBackend) reconcileClusterMetadata(ctx context.Context) error {
	clusters, err := b.MgmtClient.ListClusters(ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		return fmt.Errorf("could get cluster list: %w", err)
	}

	eg, egCtx := errgroup.WithContext(ctx)

	for _, cluster := range clusters.Items {
		hasLogging := slices.ContainsFunc(cluster.Metadata.Capabilities, func(c *opnicorev1.ClusterCapability) bool {
			return c.Name == wellknown.CapabilityLogs
		})

		if !hasLogging {
			continue
		}

		cluster := cluster

		eg.Go(func() error {
			resp, err := b.OpensearchManager.Indices.GetDocument(egCtx, multiclusterrolebinding.ClusterMetadataIndexName, cluster.Id)
			if err != nil {
				b.Logger.With(
					"cluster", cluster.Id,
					zap.Error(err),
				).Errorf("could not get metadata document for cluster")
				return nil
			}
			defer resp.Body.Close()

			if resp.IsError() {
				if resp.StatusCode != http.StatusNotFound {
					b.Logger.With(
						"cluster", cluster.Id,
						zap.Error(err),
					).Errorf("could not get metadata document for cluster")
					return nil
				}

				b.Logger.With(
					"cluster", cluster.Id,
				).Debug("no metadata found for cluster")

				if err := b.addClusterMetadata(ctx, cluster.Reference()); err != nil {
					b.Logger.With(
						"cluster", cluster.Id,
						zap.Error(err),
					).Errorf("could not reconcile cluster metadat")
					return nil
				}
			} else {
				respDoc := &types.ClusterMetadataDoc{}

				err := json.NewDecoder(resp.Body).Decode(respDoc)
				if err != nil {
					return fmt.Errorf("could not decode metadata document: %w", err)
				}

				if err := b.reconcileClusterDoc(egCtx, respDoc, cluster); err != nil {
					b.Logger.With(
						"cluster", cluster.Id,
						zap.Error(err),
					).Errorf("could not reconcile cluster metadata")
					return nil
				}
			}

			return nil
		})
	}

	b.Logger.Infof("reconciled logging cluster metadata")

	return nil
}

func (b *LoggingBackend) addClusterMetadata(ctx context.Context, ref *opnicorev1.Reference) error {
	cluster, err := b.MgmtClient.GetCluster(ctx, ref)
	if err != nil {
		return fmt.Errorf("could not get data for cluster '%s': %w", ref.Id, err)
	}

	clusterName, found := cluster.Metadata.Labels[opnicorev1.NameLabel]
	if !found {
		clusterName = cluster.Id
	}

	resp, err := b.OpensearchManager.Indices.AddDocument(ctx, multiclusterrolebinding.ClusterMetadataIndexName, cluster.Id, opensearchutil.NewJSONReader(map[string]string{
		"id":   cluster.Id,
		"name": clusterName,
	}))
	if err != nil {
		return fmt.Errorf("could not add cluster '%s' to metadata index: %w", clusterName, err)
	}
	defer resp.Body.Close()
	if resp.IsError() {
		errMsg, err := io.ReadAll(resp.Body)

		if err != nil {
			return fmt.Errorf("could not add cluster '%s' to metadata index: %w", clusterName, err)
		}

		return fmt.Errorf("could not add cluster '%s' to metadata index: %s", clusterName, errMsg)
	}

	b.Logger.With(
		"cluster", ref.Id,
	).Infof("added cluster to metadata index")

	return nil
}

func (b *LoggingBackend) updateClusterMetadata(ctx context.Context, event *managementv1.WatchEvent) error {
	clusterId := event.Cluster.Id

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

	if b.OpensearchManager.Client == nil {
		b.Logger.With(
			"cluster", clusterId,
		).Warnf("plugin has nil opensearch client, doing nothing")
		return nil
	}

	resp, err := b.OpensearchManager.Indices.UpdateDocument(ctx, multiclusterrolebinding.ClusterMetadataIndexName, clusterId, opensearchutil.NewJSONReader(
		types.MetadataUpdate{
			Document: types.ClusterMetadataDocUpdate{
				Name: newName,
			},
		},
	))

	if err != nil {
		b.Logger.With(zap.Error(err)).Errorf("failed to update cluster in metadata index")
		return nil
	}
	defer resp.Body.Close()

	if resp.IsError() {
		errMsg, err := io.ReadAll(resp.Body)

		if err != nil {
			b.Logger.With(zap.Error(err)).Errorf("failed to read response body")
			return nil
		}

		b.Logger.With(
			"cluster", clusterId,
			"newName", newName,
			zap.Error(errors.New(string(errMsg))),
		).Errorf("failed to update cluster to opensearch cluster metadata index")
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
