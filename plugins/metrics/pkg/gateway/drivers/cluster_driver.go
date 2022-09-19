package drivers

import (
	"errors"
	"fmt"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
)

type ClusterDriver interface {
	cortexops.CortexOpsServer
	// Unique name of the driver
	Name() string
	// ShouldDisableNode is called during node sync for nodes which otherwise
	// have this capability enabled. If this function returns an error, the
	// node will be set to disabled instead, and the error will be logged.
	ShouldDisableNode(*corev1.Reference) error
}

var (
	clusterDrivers       map[string]ClusterDriver
	failedClusterDrivers map[string]string
)

func init() {
	ResetClusterDrivers()
}

func RegisterClusterDriver(driver ClusterDriver) {
	if _, ok := clusterDrivers[driver.Name()]; ok {
		panic("driver already exists: " + driver.Name())
	}
	clusterDrivers[driver.Name()] = driver
}

func LogClusterDriverFailure(name string, err error) {
	failedClusterDrivers[name] = err.Error()
}

func GetClusterDriver(name string) (ClusterDriver, error) {
	driver, ok := clusterDrivers[name]
	if !ok {
		if failureMsg, ok := failedClusterDrivers[name]; ok {
			return nil, errors.New(failureMsg)
		}
		return nil, fmt.Errorf("driver not found")
	}
	return driver, nil
}

func ResetClusterDrivers() {
	clusterDrivers = make(map[string]ClusterDriver)
	failedClusterDrivers = make(map[string]string)
}

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
