package gateway

import (
	"go.uber.org/zap"

	"github.com/rancher/opni/plugins/metrics/pkg/gateway/drivers"
)

func (p *Plugin) configureCortexManagement() {
	driverName := p.config.Get().Spec.Cortex.Management.ClusterDriver
	if driverName == "" {
		p.logger.Warn("no cluster driver configured")
	}

	builder, ok := drivers.GetClusterDriverBuilder(driverName)
	if !ok {
		p.logger.With(
			"driver", driverName,
		).Error("unknown cluster driver, using fallback noop driver")

		builder, ok = drivers.GetClusterDriverBuilder("noop")
		if !ok {
			panic("bug: noop cluster driver not found")
		}
	}

	driver, err := builder(p.ctx)
	if err != nil {
		p.logger.With(
			"driver", driverName,
			zap.Error(err),
		).Error("failed to initialize cluster driver")
		return
	}

	p.clusterDriver.Set(driver)
}
