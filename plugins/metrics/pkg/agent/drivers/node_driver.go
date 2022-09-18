package drivers

import (
	"errors"
	"fmt"

	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
)

type MetricsNodeDriver interface {
	Name() string
	ConfigureNode(*node.MetricsCapabilityConfig) error
}

var (
	nodeDrivers       map[string]MetricsNodeDriver
	failedNodeDrivers map[string]string
)

func init() {
	ResetNodeDrivers()
}

func RegisterNodeDriver(driver MetricsNodeDriver) {
	if _, ok := nodeDrivers[driver.Name()]; ok {
		panic("driver already exists: " + driver.Name())
	}
	nodeDrivers[driver.Name()] = driver
}

func LogNodeDriverFailure(name string, err error) {
	failedNodeDrivers[name] = err.Error()
}

func GetNodeDriver(name string) (MetricsNodeDriver, error) {
	driver, ok := nodeDrivers[name]
	if !ok {
		if failureMsg, ok := failedNodeDrivers[name]; ok {
			return nil, errors.New(failureMsg)
		}
		return nil, fmt.Errorf("driver not found")
	}
	return driver, nil
}

func ResetNodeDrivers() {
	nodeDrivers = make(map[string]MetricsNodeDriver)
	failedNodeDrivers = make(map[string]string)
}
