package types

import (
	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/metrics/collector"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/pkg/plugins/apis/system"
)

type (
	HTTPAPIExtensionPlugin       = apiextensions.HTTPAPIExtensionClient
	ManagementAPIExtensionPlugin = apiextensions.ManagementAPIExtensionClient
	StreamAPIExtensionPlugin     = apiextensions.StreamAPIExtensionClient
	UnaryAPIExtensionPlugin      = apiextensions.UnaryAPIExtensionClient
	CapabilityBackendPlugin      = capabilityv1.BackendClient
	CapabilityNodePlugin         = capabilityv1.NodeClient
	BackendHealthPlugin          = controlv1.BackendHealthClient
	MetricsPlugin                = collector.RemoteCollector
	SystemPlugin                 = system.SystemPluginServer
)
