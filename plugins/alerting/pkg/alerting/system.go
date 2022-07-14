package alerting

import (
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/plugins/apis/system"
)

func (p *Plugin) UseManagementAPI(client managementv1.ManagementClient) {

}

func (p *Plugin) UseAPIExtensions(intf system.ExtensionClientInterface) {

}
