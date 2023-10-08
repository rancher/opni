package gateway

import (
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/util"
)

// ManagementServices implements managementext.ManagementAPIExtension.
func (p *Plugin) ManagementServices(s managementext.ServiceController) []util.ServicePackInterface {
	p.serviceCtrl.C() <- s
	return p.managementServices
}
