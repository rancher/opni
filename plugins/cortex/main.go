package main

import (
	"context"

	"github.com/rancher/opni-monitoring/pkg/capabilities/wellknown"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	gatewayext "github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions/gateway"
	managementext "github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/capability"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/system"
	"github.com/rancher/opni-monitoring/pkg/plugins/meta"
	"github.com/rancher/opni-monitoring/pkg/waitctx"
	"github.com/rancher/opni-monitoring/plugins/cortex/pkg/apis/cortexadmin"
	"github.com/rancher/opni-monitoring/plugins/cortex/pkg/cortex"
)

func main() {
	scheme := meta.NewScheme()
	ctx, ca := context.WithCancel(waitctx.FromContext(context.Background()))
	defer ca()
	p := cortex.NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(gatewayext.GatewayAPIExtensionPluginID, gatewayext.NewPlugin(p))
	scheme.Add(managementext.ManagementAPIExtensionPluginID,
		managementext.NewPlugin(&cortexadmin.CortexAdmin_ServiceDesc, p))
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewPlugin(wellknown.CapabilityMetrics, p))
	plugins.Serve(scheme)
}
