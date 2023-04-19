package drivers

import (
	"context"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/util/notifier"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"

	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
)

type MetricsNodeDriver interface {
	ConfigureNode(nodeId string, conf *node.MetricsCapabilityConfig)
	DiscoverPrometheuses(context.Context, string) ([]*remoteread.DiscoveryEntry, error)
	ConfigureRuleGroupFinder(config *v1beta1.RulesSpec) notifier.Finder[rules.RuleGroup]
}

var NodeDrivers = driverutil.NewDriverCache[MetricsNodeDriver]()

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
