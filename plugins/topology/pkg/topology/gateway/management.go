package gateway

import (
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/plugins/topology/pkg/topology/gateway/drivers"
)

func (p *Plugin) configureTopologyManagement() {
	drivers.ResetClusterDrivers()

	if kcd, err := drivers.NewTopologyManagerClusterDriver(); err == nil {
		drivers.RegisterClusterDriver(kcd)
	} else {
		drivers.LogClusterDriverFailure(kcd.Name(), err) // Name() is safe to call on a nil pointer
	}
	name := "topology-manager"
	driver, err := drivers.GetClusterDriver(name)
	if err != nil {
		p.logger.With(
			"driver", name,
			zap.Error(err),
		).Error("failed to load cluster driver, using fallback no-op driver")
		driver = &drivers.NoopClusterDriver{}
	}
	p.clusterDriver.Set(driver)
}
