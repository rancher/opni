package drivers

import (
	"errors"
	"fmt"
	"sync"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
)

type ClusterDriver interface {
	alertops.AlertingOpsServer
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

func RegisterPersistentClusterDriver(driverFunc func() ClusterDriver) {
	lock.Lock()
	defer lock.Unlock()

	persistentDrivers = append(persistentDrivers, driverFunc)
}

func LogClusterDriverFailure(name string, err error) {
	lock.Lock()
	defer lock.Unlock()

	failedClusterDrivers[name] = err.Error()
}

func GetClusterDriver(name string) (ClusterDriver, error) {
	lock.Lock()
	defer lock.Unlock()

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
	lock.Lock()
	clusterDrivers = make(map[string]ClusterDriver)
	failedClusterDrivers = make(map[string]string)
	lock.Unlock()
	for _, driverFunc := range persistentDrivers {
		RegisterClusterDriver(driverFunc())
	}
}

type NoopClusterDriver struct {
	alertops.UnimplementedAlertingOpsServer
}

func (d *NoopClusterDriver) Name() string {
	return "noop"
}

func (d *NoopClusterDriver) ShouldDisableNode(*corev1.Reference) error {
	// the noop driver will never forcefully disable a node
	return nil
}

var _ ClusterDriver = &NoopClusterDriver{}
