package drivers

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"github.com/samber/lo"
)

type MetricsNodeDriver interface {
	Name() string
	ConfigureNode(*node.MetricsCapabilityConfig)
}

var (
	lock              = &sync.Mutex{}
	nodeDrivers       map[string]MetricsNodeDriver
	failedNodeDrivers map[string]string
)

func init() {
	ResetNodeDrivers()
}

func RegisterNodeDriver(driver MetricsNodeDriver) {
	lock.Lock()
	defer lock.Unlock()

	if _, ok := nodeDrivers[driver.Name()]; ok {
		panic("driver already exists: " + driver.Name())
	}
	nodeDrivers[driver.Name()] = driver
}

func LogNodeDriverFailure(name string, err error) {
	lock.Lock()
	defer lock.Unlock()

	failedNodeDrivers[name] = err.Error()
}

func GetNodeDriver(name string) (MetricsNodeDriver, error) {
	lock.Lock()
	defer lock.Unlock()

	driver, ok := nodeDrivers[name]
	if !ok {
		if failureMsg, ok := failedNodeDrivers[name]; ok {
			return nil, errors.New(failureMsg)
		}
		return nil, fmt.Errorf("driver not found")
	}
	return driver, nil
}

func ListNodeDrivers() []string {
	lock.Lock()
	defer lock.Unlock()

	return lo.Keys(nodeDrivers)
}

func ResetNodeDrivers() {
	lock.Lock()
	defer lock.Unlock()

	nodeDrivers = make(map[string]MetricsNodeDriver)
	failedNodeDrivers = make(map[string]string)
}

func NewListenerFunc(ctx context.Context, fn func(cfg *node.MetricsCapabilityConfig)) chan<- *node.MetricsCapabilityConfig {
	listenerC := make(chan *node.MetricsCapabilityConfig, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case cfg := <-listenerC:
				fn(cfg)
			}
		}
	}()
	return listenerC
}
