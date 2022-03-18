package main

import (
	"context"

	"github.com/rancher/opni-monitoring/pkg/capabilities/wellknown"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	gatewayext "github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions/gateway"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/capability"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/system"
	"github.com/rancher/opni-monitoring/pkg/plugins/meta"
	"github.com/rancher/opni-monitoring/pkg/waitctx"
	"github.com/rancher/opni-monitoring/plugins/logging/pkg/logging"
	opniv1beta2 "github.com/rancher/opni/apis/v1beta2"
)

func main() {
	scheme := meta.NewScheme()
	ctx, ca := context.WithCancel(waitctx.FromContext(context.Background()))
	defer ca()

	opniCluster := &opniv1beta2.OpensearchClusterRef{
		Name:      "opni",
		Namespace: "opni-cluster-system",
	}

	p := logging.NewPlugin(
		ctx,
		logging.WithNamespace("opni-cluster-system"),
		logging.WithOpensearchCluster(opniCluster),
	)

	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(gatewayext.GatewayAPIExtensionPluginID, gatewayext.NewPlugin(p))
	scheme.Add(capability.CapabilityBackendPluginID, capability.NewPlugin(wellknown.CapabilityLogs, p))
	plugins.Serve(scheme)
}
