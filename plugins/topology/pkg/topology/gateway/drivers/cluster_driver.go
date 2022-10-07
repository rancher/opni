package drivers

import (
	"sync"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/topology/pkg/apis/orchestrator"
)

type ClusterDriver interface {
	orchestrator.TopologyOrchestratorServer
	// Unique name of the driver
	Name() string
	// ShouldDisableNode is called during node sync for nodes which otherwise
	// have this capability enabled. If this function returns an error, the
	// node will be set to disabled instead, and the error will be logged.
	ShouldDisableNode(*corev1.Reference) error
}

var (
	lock                 = &sync.Mutex{}
	clusterDrivers       map[string]ClusterDriver
	persistentDrivers    []func() ClusterDriver
	failedClusterDrivers map[string]string
)

func init() {
	ResetClusterDrivers()
}

func RegisterClusterDriver(driver ClusterDriver) {
	lock.Lock()
	defer lock.Unlock()

	if _, ok := clusterDrivers[driver.Name()]; ok {
		panic("driver already exists: " + driver.Name())
	}
	clusterDrivers[driver.Name()] = driver
}

func ResetClusterDrivers() {
	lock.Lock()
	clusterDrivers = make(map[string]ClusterDriver)
	failedClusterDrivers = make(map[string]string)
	lock.Unlock()
	for _, driverFunc := range persistentDrivers {
		RegisterClusterDriver(driverFunc())
	}
}

type NoopClusterDriver struct {
	// cortexops.UnimplementedCortexOpsServer
}

func (d *NoopClusterDriver) Name() string {
	return "noop"
}

func (d *NoopClusterDriver) ShouldDisableNode(*corev1.Reference) error {
	// the noop driver will never forcefully disable a node
	return nil
}
