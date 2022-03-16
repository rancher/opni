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
	opniv2beta1 "github.com/rancher/opni/apis/v2beta1"
)

func main() {
	scheme := meta.NewScheme()
	ctx, ca := context.WithCancel(waitctx.FromContext(context.Background()))
	defer ca()

	opniCluster := &opniv2beta1.OpensearchClusterRef{
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
