package types

import (
	"github.com/rancher/opni/pkg/metrics/collector"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/plugins/apis/capability"
	"github.com/rancher/opni/pkg/plugins/apis/system"
)

type (
	GatewayAPIExtensionPlugin    = apiextensions.GatewayAPIExtensionClient
	ManagementAPIExtensionPlugin = apiextensions.ManagementAPIExtensionClient
	StreamAPIExtensionPlugin     = apiextensions.StreamAPIExtensionClient
	UnaryAPIExtensionPlugin      = apiextensions.UnaryAPIExtensionClient
	CapabilityBackendPlugin      = capability.BackendClient
	MetricsPlugin                = collector.RemoteCollector
	SystemPlugin                 = system.SystemPluginServer
)
