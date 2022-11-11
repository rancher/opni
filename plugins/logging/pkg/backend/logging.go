package backend

import (
	"context"
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
}

type LoggingBackendConfig struct {
	Logger              *zap.SugaredLogger             `validate:"required"`
	OpensearchCluster   *opnimeta.OpensearchClusterRef `validate:"required"`
	StorageBackend      storage.Backend                `validate:"required"`
	MgmtClient          managementv1.ManagementClient  `validate:"required"`
	NodeManagerClient   capabilityv1.NodeManagerClient `validate:"required"`
	UninstallController *task.Controller               `validate:"required"`
	ClusterDriver       drivers.ClusterDriver          `validate:"required"`
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

func (b *LoggingBackend) Info(context.Context, *emptypb.Empty) (*capabilityv1.Details, error) {
	return &capabilityv1.Details{
		Name:    wellknown.CapabilityLogs,
		Source:  "plugin_logging",
		Drivers: drivers.ListClusterDrivers(),
	}, nil
}
