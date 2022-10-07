package gateway

import (
	"github.com/rancher/opni/plugins/topology/pkg/topology/gateway/drivers"
	"go.uber.org/zap"
)

func (p *Plugin) configureTopologyManagement() {
	drivers.ResetClusterDrivers()

	// if kcd, err := drivers.NewOpniManagerClusterDriver(); err == nil {
	// 	drivers.RegisterClusterDriver(kcd)
	// } else {
	// 	drivers.LogClusterDriverFailure(kcd.Name(), err)
	// }

	// TODO : acquire the real driver for topology
	name := "topology-driver-name"
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
