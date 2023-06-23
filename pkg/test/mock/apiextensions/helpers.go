package mock_apiextensions

import apiextensions "github.com/rancher/opni/pkg/plugins/apis/apiextensions"

type MockManagementAPIExtensionServerImpl struct {
	apiextensions.UnsafeManagementAPIExtensionServer
	*MockManagementAPIExtensionServer
}
