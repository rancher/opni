package drivers

import (
	"context"

	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/rules"
)

type ConfigPropagator interface {
	ConfigureNode(nodeId string, conf *node.AlertingCapabilityConfig) error
}

type NodeDriver interface {
	ConfigPropagator
	DiscoverRules(ctx context.Context) (*rules.RuleManifest, error)
}

var NodeDrivers = driverutil.NewDriverCache[NodeDriver]()
