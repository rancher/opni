package drivers

import (
	"errors"
	"fmt"

	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexops"
)

type ClusterDriver interface {
	Name() string
	cortexops.CortexOpsServer
}

var (
	drivers       map[string]ClusterDriver
	failedDrivers map[string]string
)

func init() {
	ResetClusterDrivers()
}

func RegisterClusterDriver(driver ClusterDriver) {
	if _, ok := drivers[driver.Name()]; ok {
		panic("driver already exists: " + driver.Name())
	}
	drivers[driver.Name()] = driver
}

func LogDriverFailure(name string, err error) {
	failedDrivers[name] = err.Error()
}

func GetClusterDriver(name string) (ClusterDriver, error) {
	driver, ok := drivers[name]
	if !ok {
		if failureMsg, ok := failedDrivers[name]; ok {
			return nil, errors.New(failureMsg)
		}
		return nil, fmt.Errorf("driver not found")
	}
	return driver, nil
}

func ResetClusterDrivers() {
	drivers = make(map[string]ClusterDriver)
	failedDrivers = make(map[string]string)
}
