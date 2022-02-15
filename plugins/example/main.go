package main

import (
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni-monitoring/pkg/plugins/apis/system"
	"github.com/rancher/opni-monitoring/pkg/plugins/meta"
	example "github.com/rancher/opni-monitoring/plugins/example/pkg"
)

func main() {
	scheme := meta.NewScheme()
	p := &example.ExamplePlugin{
		Logger: logger.NewForPlugin(),
	}
	scheme.Add(apiextensions.ManagementAPIExtensionPluginID,
		apiextensions.NewPlugin(&example.ExampleAPIExtension_ServiceDesc, p))
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))

	plugins.Serve(scheme)
}
