package drivers

import (
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
)

type ConfigPropagator interface {
	ConfigureNode(nodeId string, conf *node.AlertingCapabilityConfig) error
}

type NodeDriver interface {
	// TODO : implement a real rule discovery struct
	ConfigureRuleDiscovery(conf *node.AlertingCapabilityConfig) struct{}
}

var NodeDrivers = driverutil.NewDriverCache[NodeDriver]()
