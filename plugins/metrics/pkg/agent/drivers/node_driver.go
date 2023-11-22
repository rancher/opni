package drivers

import (
	"context"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/util/notifier"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
)

type MetricsNodeConfigurator interface {
	ConfigureNode(nodeId string, conf *node.MetricsCapabilityStatus) error
}

type MetricsNodeDriver interface {
	MetricsNodeConfigurator
	DiscoverPrometheuses(context.Context, string) ([]*remoteread.DiscoveryEntry, error)
	ConfigureRuleGroupFinder(config *v1beta1.RulesSpec) notifier.Finder[rules.RuleGroup]
}

var NodeDrivers = driverutil.NewCache[MetricsNodeDriver]()
