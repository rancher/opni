package gateway

import (
	"context"
	"errors"
	"github.com/opensearch-project/opensearch-go/opensearchutil"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/opensearch/opensearch/types"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"io"
	"os"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/task"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) UseManagementAPI(client managementv1.ManagementClient) {
	p.mgmtApi.Set(client)
	cfg, err := client.GetConfig(context.Background(), &emptypb.Empty{}, grpc.WaitForReady(true))
	if err != nil {
		p.logger.With(
			"err", err,
		).Error("failed to get config")
		os.Exit(1)
	}

	objectList, err := machinery.LoadDocuments(cfg.Documents)
	if err != nil {
		p.logger.With(
			"err", err,
		).Error("failed to load config")
		os.Exit(1)
	}

	machinery.LoadAuthProviders(p.ctx, objectList)

	objectList.Visit(func(config *v1beta1.GatewayConfig) {
		backend, err := machinery.ConfigureStorageBackend(p.ctx, &config.Spec.Storage)
		if err != nil {
			p.logger.With(
				"err", err,
			).Error("failed to configure storage backend")
			os.Exit(1)
		}
		p.storageBackend.Set(backend)
	})
	<-p.ctx.Done()
}

func (p *Plugin) UseNodeManagerClient(client capabilityv1.NodeManagerClient) {
	p.nodeManagerClient.Set(client)
	<-p.ctx.Done()
}

func (p *Plugin) UseKeyValueStore(client system.KeyValueStoreClient) {
	ctrl, err := task.NewController(p.ctx, "uninstall", system.NewKVStoreClient[*opnicorev1.TaskStatus](client), &UninstallTaskRunner{
		storageNamespace:  p.storageNamespace,
		opensearchManager: p.opensearchManager,
		k8sClient:         p.k8sClient,
		storageBackend:    p.storageBackend,
		logger:            p.logger.Named("uninstaller"),
	})
	if err != nil {
		p.logger.With(
			"err", err,
		).Error("failed to create task controller")
		os.Exit(1)
	}

	p.uninstallController.Set(ctrl)
	<-p.ctx.Done()
}

func (p *Plugin) watchGlobalCluster(client managementv1.ManagementClient) {
	clusterClient, err := client.WatchClusters(p.ctx, &managementv1.WatchClustersRequest{})
	if err != nil {
		p.logger.Errorf("failed to watch clusters, exiting")
		os.Exit(1)
	}

	p.logger.Infof("watching cluster events")

	for {
		select {
		case <-p.ctx.Done():
			p.logger.Infof("context cancelled, stopping cluster event watcher")
			return
		default:
			event, err := clusterClient.Recv()
			if err != nil {
				p.logger.Errorf("failed to receive cluster event: %s", err)
				continue
			}

			clusterId := event.Cluster.Id

			hasLogging := slices.ContainsFunc(event.Cluster.Metadata.Capabilities, func(c *opnicorev1.ClusterCapability) bool {
				return c.Name == wellknown.CapabilityLogs
			})

			if !hasLogging {
				p.logger.With(
					"cluster", clusterId,
				).Debug("cluster does not have logging, ignoring cluster event")
				continue
			}

			// todo: we probably want to add a diff or the old cluster value to avoid updating opensearch each time a logging cluster is updated
			// this is not necessarily a new name since we only know the current name, so we must always treat it as new
			newName, hasName := event.Cluster.Metadata.Labels[opnicorev1.NameLabel]
			if !hasName {
				newName = event.Cluster.Id
			}

			if p.opensearchManager.Client == nil {
				p.logger.With(
					"cluster", clusterId,
				).Warnf("plugin has nil opensearch client, doing nothing")
				continue
			}

			switch event.Type {
			case managementv1.WatchEventType_Updated:
				p.logger.With().Infof("received cluster update event for '%s' (%s)", newName, event.Cluster.Metadata.Labels[opnicorev1.NameLabel])

				// cluster does not have a newName and could not have been renamed
				if !hasName {
					continue
				}

				p.logger.Debugf("cluster was renamed")
				resp, err := p.opensearchManager.Indices.UpdateDocument(p.ctx, "opni-cluster-metadata", clusterId, opensearchutil.NewJSONReader(
					types.MetadataUpdate{
						Document: types.ClusterMetadataUpdate{
							Name: newName,
						},
					},
				))

				if err != nil {
					p.logger.Errorf("failed to update cluster in metadata index: %s", err)
					continue
				}

				if resp.IsError() {
					errMsg, err := io.ReadAll(resp.Body)

					if err != nil {
						p.logger.Errorf("failed to read response body: %s", err)
					}

					p.logger.With(
						"cluster", clusterId,
						"newName", newName,
						zap.Error(errors.New(string(errMsg))),
					).Errorf("failed to update cluster to opensearch cluster newName index")
				}
				resp.Body.Close()
			}
		}
	}
}
