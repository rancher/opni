package gateway

import (
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/util"
)

// ManagementServices implements managementext.ManagementAPIExtension.
func (p *Plugin) ManagementServices(s managementext.ServiceController) []util.ServicePackInterface {
	servicesC := make(chan util.ServicePackInterface)
	defer close(servicesC)
	var services []util.ServicePackInterface

	registrarC := <-p.mgmtServiceRegistrarC
	registrarC <- func(spi util.ServicePackInterface) managementext.SingleServiceController {
		servicesC <- spi // if called outside Activate, this should panic
		return managementext.NewSingleServiceController(s, spi.ServiceName())
	}

LOOP:
	for {
		select {
		case spi := <-servicesC:
			services = append(services, spi)
		case <-registrarC:
			break LOOP
		case <-p.ctx.Done():
			return nil
		}
	}

	return services
}
