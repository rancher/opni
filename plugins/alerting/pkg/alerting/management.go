package alerting

import (
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/drivers"
	"go.uber.org/zap"
)

func (p *Plugin) configureAlertManagerConfiguration(opts ...drivers.AlertingManagerDriverOption) {
	// load default cluster drivers
	drivers.ResetClusterDrivers()
	if kcd, err := drivers.NewAlertingManagerDriver(opts...); err == nil {
		drivers.RegisterClusterDriver(kcd)
	} else {
		drivers.LogClusterDriverFailure(kcd.Name(), err) // Name() is safe to call on a nil pointer
	}

	name := "alerting-mananger"
	driver, err := drivers.GetClusterDriver(name)
	if err != nil {
		p.Logger.With(
			"driver", name,
			zap.Error(err),
		).Error("failed to load cluster driver, using fallback no-op driver")
		driver = &drivers.NoopClusterDriver{}
	}
	p.opsNode.ClusterDriver.Set(driver)
}
