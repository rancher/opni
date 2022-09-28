package backend

import (
	"context"
	"sync"

	osclient "github.com/opensearch-project/opensearch-go"
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/plugins/logging/pkg/apis/node"
	loggingutil "github.com/rancher/opni/plugins/logging/pkg/util"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type LoggingBackend struct {
	capabilityv1.UnsafeBackendServer
	node.UnsafeNodeLoggingCapabilityServer
	LoggingBackendConfig
	loggingutil.Initializer

	nodeStatusMu      sync.RWMutex
	desiredNodeSpecMu sync.RWMutex
}

type LoggingBackendConfig struct {
	K8sClient           client.Client                  `validate:"required"`
	Logger              *zap.SugaredLogger             `validate:"required"`
	Namespace           string                         `validate:"required"`
	OpensearchClient    *osclient.Client               `validate:"required"`
	OpensearchCluster   *opnimeta.OpensearchClusterRef `validate:"required"`
	StorageBackend      storage.Backend                `validate:"required"`
	MgmtClient          managementv1.ManagementClient  `validate:"required"`
	NodeManagerClient   capabilityv1.NodeManagerClient `validate:"required"`
	UninstallController *task.Controller               `validate:"required"`
}

var _ node.NodeLoggingCapabilityServer = (*LoggingBackend)(nil)

// TODO: set up watches on underlying k8s objects to dynamically request a sync
func (b *LoggingBackend) Initialize(conf LoggingBackendConfig) {
	b.InitOnce(func() {
		if err := loggingutil.Validate.Struct(conf); err != nil {
			panic(err)
		}
		b.LoggingBackendConfig = conf
	})
}

func (b *LoggingBackend) Info(context.Context, *emptypb.Empty) (*capabilityv1.InfoResponse, error) {
	return &capabilityv1.InfoResponse{
		CapabilityName: wellknown.CapabilityLogs,
	}, nil
}
