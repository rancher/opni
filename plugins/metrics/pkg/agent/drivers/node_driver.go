package drivers

import (
	"context"
	"sync"

	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"

	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"github.com/samber/lo"
)

type MetricsNodeDriverBuilder = func() (MetricsNodeDriver, error)

type MetricsNodeDriver interface {
	ConfigureNode(nodeId string, conf *node.MetricsCapabilityConfig)
	DiscoverPrometheuses(context.Context, string) ([]*remoteread.DiscoveryEntry, error)
}

var (
	lock               = &sync.Mutex{}
	nodeDriverBuilders = make(map[string]MetricsNodeDriverBuilder)
)

func RegisterNodeDriverBuilder(name string, fn MetricsNodeDriverBuilder) {
	lock.Lock()
	defer lock.Unlock()

	nodeDriverBuilders[name] = fn
}

func UnregisterNodeDriverBuilder(name string) {
	lock.Lock()
	defer lock.Unlock()

	delete(nodeDriverBuilders, name)
}

func GetNodeDriverBuilder(name string) (MetricsNodeDriverBuilder, bool) {
	lock.Lock()
	defer lock.Unlock()

	driver, ok := nodeDriverBuilders[name]
	return driver, ok
}

func ListNodeDrivers() []string {
	lock.Lock()
	defer lock.Unlock()

	return lo.Keys(nodeDriverBuilders)
}

type ConfigureNodeArgs struct {
	NodeId string
	Config *node.MetricsCapabilityConfig
}

func NewListenerFunc(ctx context.Context, fn func(nodeId string, cfg *node.MetricsCapabilityConfig)) chan<- ConfigureNodeArgs {
	listenerC := make(chan ConfigureNodeArgs, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case args := <-listenerC:
				fn(args.NodeId, args.Config)
			}
		}
	}()
	return listenerC
}
