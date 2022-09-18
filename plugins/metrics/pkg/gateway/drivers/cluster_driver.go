package drivers

import (
	"errors"
	"fmt"

	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
)

type ClusterDriver interface {
	Name() string
	cortexops.CortexOpsServer
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
