package drivers

import (
	"context"
	"fmt"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/util/notifier"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
)

type MetricsNodeConfigurator interface {
	ConfigureNode(nodeId string, conf *node.MetricsCapabilityConfig) error
}

type MetricsNodeDriver interface {
	MetricsNodeConfigurator
	DiscoverPrometheuses(context.Context, string) ([]*remoteread.DiscoveryEntry, error)
	ConfigureRuleGroupFinder(config *v1beta1.RulesSpec) notifier.Finder[rules.RuleGroup]
}

var NodeDrivers = driverutil.NewDriverCache[MetricsNodeDriver]()

type ConfigurationError struct {
	NodeId string
	Cfg    *node.MetricsCapabilityConfig
	Err    error
}

func (e *ConfigurationError) Error() string {
	return fmt.Errorf("error configuring node %s: %w", e.NodeId, e.Err).Error()
}
