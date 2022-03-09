package main

import (
	"context"

	"github.com/rancher/opni-monitoring/pkg/plugins"
	gatewayext "github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions/gateway"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/system"
	"github.com/rancher/opni-monitoring/pkg/plugins/meta"
	"github.com/rancher/opni-monitoring/pkg/waitctx"
	"github.com/rancher/opni-monitoring/plugins/cortex/pkg/cortex"
)

func main() {
	scheme := meta.NewScheme()
	ctx, ca := context.WithCancel(waitctx.FromContext(context.Background()))
	defer ca()
	p := cortex.NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(gatewayext.GatewayAPIExtensionPluginID, gatewayext.NewPlugin(p))
	plugins.Serve(scheme)
}
