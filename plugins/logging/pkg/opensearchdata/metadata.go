package opensearchdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/opensearch-project/opensearch-go/opensearchutil"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/opensearch/opensearch/types"
	"github.com/rancher/opni/pkg/resources/multiclusterrolebinding"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"io"
	"net/http"
)

func (m *Manager) AddClusterMetadata(ctx context.Context, cluster *opnicorev1.Cluster) error {
	clusterName, found := cluster.Metadata.Labels[opnicorev1.NameLabel]
	if !found {
		clusterName = cluster.Id
	}

	resp, err := m.Indices.AddDocument(ctx, multiclusterrolebinding.ClusterMetadataIndexName, cluster.Id, opensearchutil.NewJSONReader(map[string]string{
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

	return nil
}

func (m *Manager) UpdateClusterMetadata(ctx context.Context, ref *opnicorev1.Reference, update types.ClusterMetadataDocUpdate) error {
	if m.Client == nil {
		return fmt.Errorf("manager has nil opensearch client, doing nothing")
	}

	resp, err := m.Indices.UpdateDocument(ctx, multiclusterrolebinding.ClusterMetadataIndexName, ref.Id, opensearchutil.NewJSONReader(
		types.MetadataUpdate{
			Document: update,
		},
	))

	if err != nil {
		return fmt.Errorf("failed to update cluster in metadata index")
	}
	defer resp.Body.Close()

	if resp.IsError() {
		errMsg, err := io.ReadAll(resp.Body)

		if err != nil {
			return fmt.Errorf("failed to read response body")
		}

		return fmt.Errorf("failed to update ref to opensearch cluster metadata index: %w", errors.New(string(errMsg)))
	}

	return nil
}

func (m *Manager) reconcileClusterDoc(ctx context.Context, doc *types.ClusterMetadataDoc, cluster *opnicorev1.Cluster) error {
	if currentName := cluster.Metadata.Labels[opnicorev1.NameLabel]; doc.Name != currentName {
		m.logger.With(
			"cluster", cluster.Id,
		).Debug("cluster name has changed")

		resp, err := m.Indices.UpdateDocument(ctx, multiclusterrolebinding.ClusterMetadataIndexName, cluster.Id, opensearchutil.NewJSONReader(
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

func (m *Manager) ReconcileClusterMetadata(ctx context.Context, clusters []*opnicorev1.Cluster) error {
	eg, egCtx := errgroup.WithContext(ctx)

	for _, cluster := range clusters {
		hasLogging := slices.ContainsFunc(cluster.Metadata.Capabilities, func(c *opnicorev1.ClusterCapability) bool {
			return c.Name == wellknown.CapabilityLogs
		})

		if !hasLogging {
			continue
		}

		cluster := cluster

		eg.Go(func() error {
			resp, err := m.Indices.GetDocument(egCtx, multiclusterrolebinding.ClusterMetadataIndexName, cluster.Id)
			if err != nil {
				m.logger.With(
					"cluster", cluster.Id,
					zap.Error(err),
				).Errorf("could not get metadata document for cluster")
				return nil
			}
			defer resp.Body.Close()

			if resp.IsError() {
				if resp.StatusCode != http.StatusNotFound {
					m.logger.With(
						"cluster", cluster.Id,
						zap.Error(err),
					).Errorf("could not get metadata document for cluster")
					return nil
				}

				m.logger.With(
					"cluster", cluster.Id,
				).Debug("no metadata found for cluster")

				if err := m.AddClusterMetadata(ctx, cluster); err != nil {
					m.logger.With(
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

				if err := m.reconcileClusterDoc(egCtx, respDoc, cluster); err != nil {
					m.logger.With(
						"cluster", cluster.Id,
						zap.Error(err),
					).Errorf("could not reconcile cluster metadata")
					return nil
				}
			}

			return nil
		})
	}

	m.logger.Infof("reconciled logging cluster metadata")

	return nil
}
