package features

import (
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/util"
)

type Feature interface {
	UseManagementAPI(managementv1.ManagementClient)
	UseAPIExtensions(system.ExtensionClientInterface)
	ManagementAPIExtensionServices() []util.ServicePackInterface
}

var Features = driverutil.NewDriverCache[Feature]()
