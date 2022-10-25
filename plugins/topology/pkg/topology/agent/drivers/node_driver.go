package drivers

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/rancher/opni/plugins/topology/pkg/apis/node"
)

type TopologyNodeDriver interface {
	Name() string
	ConfigureNode(*node.TopologyCapabilityConfig)
}

var (
	lock              = &sync.Mutex{}
	nodeDrivers       map[string]TopologyNodeDriver
	failedNodeDrivers map[string]string
)

func init() {
	ResetNodeDrivers()
}

func ResetNodeDrivers() {
	lock.Lock()
	defer lock.Unlock()
	nodeDrivers = make(map[string]TopologyNodeDriver)
	failedNodeDrivers = make(map[string]string)
}

func RegisterNodeDriver(driver TopologyNodeDriver) {
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

func GetNodeDriver(name string) (TopologyNodeDriver, error) {
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

func NewListenerFunc(ctx context.Context, fn func(cfg *node.TopologyCapabilityConfig)) chan<- *node.TopologyCapabilityConfig {
	listenerC := make(chan *node.TopologyCapabilityConfig, 1)
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
