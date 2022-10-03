package gateway

import (
	"go.uber.org/zap"

	"github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"
)

func (p *Plugin) configureCortexManagement() {
	// load default cluster drivers
	drivers.ResetClusterDrivers()
	if kcd, err := drivers.NewOpniManagerClusterDriver(); err == nil {
		drivers.RegisterClusterDriver(kcd)
	} else {
		drivers.LogClusterDriverFailure(kcd.Name(), err) // Name() is safe to call on a nil pointer
	}

	driverName := p.config.Get().Spec.Cortex.Management.ClusterDriver
	if driverName == "" {
		p.logger.Warn("no cluster driver configured")
	}

	driver, err := drivers.GetClusterDriver(driverName)
	if err != nil {
		p.logger.With(
			"driver", driverName,
			zap.Error(err),
		).Error("failed to load cluster driver, using fallback no-op driver")

		driver = &drivers.NoopClusterDriver{}
	}

	p.clusterDriver.Set(driver)
}
