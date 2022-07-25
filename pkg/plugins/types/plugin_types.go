package types

import (
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	"github.com/rancher/opni/pkg/metrics/collector"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/plugins/apis/system"
)

type (
	GatewayAPIExtensionPlugin    = apiextensions.GatewayAPIExtensionClient
	ManagementAPIExtensionPlugin = apiextensions.ManagementAPIExtensionClient
	StreamAPIExtensionPlugin     = apiextensions.StreamAPIExtensionClient
	UnaryAPIExtensionPlugin      = apiextensions.UnaryAPIExtensionClient
	CapabilityBackendPlugin      = capabilityv1.BackendClient
	MetricsPlugin                = collector.RemoteCollector
	SystemPlugin                 = system.SystemPluginServer
)
