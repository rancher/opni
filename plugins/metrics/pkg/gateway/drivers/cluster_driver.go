package drivers

import (
	"context"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ClusterDriver interface {
	ActiveConfigStore() storage.ValueStoreT[*cortexops.CapabilityBackendConfigSpec]

	// ShouldDisableNode is called during node sync for nodes which otherwise
	// have this capability enabled. If this function returns an error, the
	// node will be set to disabled instead, and the error will be logged.
	ShouldDisableNode(*corev1.Reference) error

	PartialCortexOpsServer
}

type PartialCortexOpsServer interface {
	ListPresets(context.Context, *emptypb.Empty) (*cortexops.PresetList, error)
	Status(context.Context, *emptypb.Empty) (*driverutil.InstallStatus, error)
}

var ClusterDrivers = driverutil.NewCache[ClusterDriver]()
