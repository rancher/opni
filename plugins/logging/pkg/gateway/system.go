package gateway

import (
	"context"
	"github.com/opensearch-project/opensearch-go/opensearchutil"
	"github.com/rancher/opni/pkg/opensearch/opensearch/types"
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

			docId := event.Cluster.Id

			name, hasName := event.Cluster.Metadata.Labels[opnicorev1.NameLabel]
			if !hasName {
				name = event.Cluster.Id
			}

			if p.opensearchManager.Client == nil {
				p.logger.Warnf("plugin has nil opensearch client, doing nothing")
				continue
			}

			// todo: handle a cluster rejoining (does it come in as Created or as Updated?)
			switch event.Type {
			case managementv1.WatchEventType_Created:
				p.logger.Infof("received cluster create event for '%s'", event.Cluster)
				resp, err := p.opensearchManager.Indices.AddDocument(p.ctx, "opni-cluster-metadata", docId, opensearchutil.NewJSONReader(map[string]string{
					"id":   event.Cluster.Id,
					"name": name,
				}))
				if err != nil {
					p.logger.Errorf("failed to add cluster to opensearch cluster name index: %s", err)
					continue
				}
				if resp.IsError() {
					p.logger.Errorf("failed to add cluster to opensearch cluster name index")
				}
				resp.Body.Close()
			case managementv1.WatchEventType_Updated:
				p.logger.With().Infof("received cluster update event for '%s' (%s)", name, event.Cluster.Metadata.Labels[opnicorev1.NameLabel])

				// cluster does not have a name and could not have been renamed
				if !hasName {
					continue
				}

				p.logger.Debugf("cluster was renamed")
				resp, err := p.opensearchManager.Indices.UpdateDocument(p.ctx, "opni-cluster-metadata", docId, opensearchutil.NewJSONReader(
					types.MetadataUpdate{
						Document: types.ClusterMetadataUpdate{
							Name: name,
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

					p.logger.Errorf("failed to add cluster to opensearch cluster name index: %s", string(errMsg))
				}
				resp.Body.Close()
			case managementv1.WatchEventType_Deleted:
				p.logger.Infof("recevied cluster delete event for '%s'", event.Cluster.Id)
				// todo: we might want to mark clusters as inactive
			}
		}
	}
}
