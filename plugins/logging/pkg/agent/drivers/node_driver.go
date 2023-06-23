package drivers

import (
	"context"

	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/plugins/logging/apis/node"
)

type LoggingNodeDriver interface {
	ConfigureNode(*node.LoggingCapabilityConfig)
}

var NodeDrivers = driverutil.NewDriverCache[LoggingNodeDriver]()

func NewListenerFunc(ctx context.Context, fn func(cfg *node.LoggingCapabilityConfig)) chan<- *node.LoggingCapabilityConfig {
	listenerC := make(chan *node.LoggingCapabilityConfig, 1)
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
