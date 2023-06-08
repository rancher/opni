package default_driver

import (
	"context"

	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/plugins/alerting/pkg/agent/drivers"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/node"
)

type Driver struct{}

func NewDriver() *Driver {
	return &Driver{}
}

var _ drivers.NodeDriver = (*Driver)(nil)

func (d *Driver) ConfigureRuleDiscovery(conf *node.AlertingCapabilityConfig) struct{} {
	return struct{}{}
}

func init() {
	drivers.NodeDrivers.Register("default_driver", func(_ context.Context, _ ...driverutil.Option) (drivers.NodeDriver, error) {
		return NewDriver(), nil
	})
}
