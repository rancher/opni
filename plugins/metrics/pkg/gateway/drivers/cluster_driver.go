package drivers

import (
	"context"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
)

type ClusterDriver interface {
	cortexops.CortexOpsServer
	// ShouldDisableNode is called during node sync for nodes which otherwise
	// have this capability enabled. If this function returns an error, the
	// node will be set to disabled instead, and the error will be logged.
	ShouldDisableNode(*corev1.Reference) error
	ConfigureOTELCollector(enable bool) (collectorAddress string, err error)
}

var ClusterDrivers = driverutil.NewDriverCache[ClusterDriver]()

type NoopClusterDriver struct {
	cortexops.UnimplementedCortexOpsServer
}

func (d *NoopClusterDriver) Name() string {
	return "noop"
}

func (d *NoopClusterDriver) ShouldDisableNode(*corev1.Reference) error {
	// the noop driver will never forcefully disable a node
	return nil
}

func (d *NoopClusterDriver) ConfigureOTELCollector(_ bool) (collectorAddress string, err error) {
	return "", nil
}

func init() {
	ClusterDrivers.Register("noop", func(context.Context, ...driverutil.Option) (ClusterDriver, error) {
		return &NoopClusterDriver{}, nil
	})
}
