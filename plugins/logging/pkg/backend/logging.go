package backend

import (
	"context"
	"errors"
	"github.com/opensearch-project/opensearch-go/opensearchutil"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/gateway"
	"github.com/rancher/opni/pkg/opensearch/opensearch/types"
	"github.com/rancher/opni/plugins/logging/pkg/opensearchdata"
	"golang.org/x/exp/slices"
	"io"
	"os"
	"sync"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/plugins/logging/pkg/apis/node"
	"github.com/rancher/opni/plugins/logging/pkg/gateway/drivers"
	loggingutil "github.com/rancher/opni/plugins/logging/pkg/util"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

type LoggingBackend struct {
	capabilityv1.UnsafeBackendServer
	node.UnsafeNodeLoggingCapabilityServer
	LoggingBackendConfig
	util.Initializer

	nodeStatusMu      sync.RWMutex
	desiredNodeSpecMu sync.RWMutex
	watcher           *gateway.ManagementWatcherHooks[*managementv1.WatchEvent]
}

type LoggingBackendConfig struct {
	Logger              *zap.SugaredLogger             `validate:"required"`
	OpensearchCluster   *opnimeta.OpensearchClusterRef `validate:"required"`
	StorageBackend      storage.Backend                `validate:"required"`
	MgmtClient          managementv1.ManagementClient  `validate:"required"`
	NodeManagerClient   capabilityv1.NodeManagerClient `validate:"required"`
	UninstallController *task.Controller               `validate:"required"`
	ClusterDriver       drivers.ClusterDriver          `validate:"required"`
	OpensearchManager   *opensearchdata.Manager        `validate:"required"`
}

var _ node.NodeLoggingCapabilityServer = (*LoggingBackend)(nil)

// TODO: set up watches on underlying k8s objects to dynamically request a sync
func (b *LoggingBackend) Initialize(conf LoggingBackendConfig) {
	b.InitOnce(func() {
		if err := loggingutil.Validate.Struct(conf); err != nil {
			panic(err)
		}
		b.LoggingBackendConfig = conf

		b.watcher = gateway.NewManagementWatcherHooks[*managementv1.WatchEvent](context.TODO())
		b.watcher.RegisterHook(func(event *managementv1.WatchEvent) bool {
			return event.Type == managementv1.WatchEventType_Updated && slices.ContainsFunc(event.Cluster.Metadata.Capabilities, func(c *opnicorev1.ClusterCapability) bool {
				return c.Name == wellknown.CapabilityLogs
			})
		}, b.updateClusterMetadata)

		go func() {
			b.watchClusterEvents(context.Background())
		}()
	})
}

func (b *LoggingBackend) Info(context.Context, *emptypb.Empty) (*capabilityv1.Details, error) {
	return &capabilityv1.Details{
		Name:    wellknown.CapabilityLogs,
		Source:  "plugin_logging",
		Drivers: drivers.ListClusterDrivers(),
	}, nil
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

	resp, err := b.OpensearchManager.Indices.UpdateDocument(ctx, "opni-cluster-metadata", clusterId, opensearchutil.NewJSONReader(
		types.MetadataUpdate{
			Document: types.ClusterMetadataUpdate{
				Name: newName,
			},
		},
	))

	if err != nil {
		b.Logger.With(zap.Error(err)).Errorf("failed to update cluster in metadata index")
		return nil
	}

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
	resp.Body.Close()

	return nil
}

func (b *LoggingBackend) watchClusterEvents(ctx context.Context) {
	clusterClient, err := b.MgmtClient.WatchClusters(ctx, &managementv1.WatchClustersRequest{})
	if err != nil {
		b.Logger.With(zap.Error(err)).Errorf("failed to watch clusters, existing")
		os.Exit(1)
	}

	b.Logger.Infof("watching cluster events")

	for {
		select {
		case <-ctx.Done():
			b.Logger.Infof("context cvancelled, stoping cluster event watcher")
			break
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
