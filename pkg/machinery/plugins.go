package machinery

import (
	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/plugins"
	pluginmeta "github.com/rancher/opni-monitoring/pkg/plugins/meta"
	"go.uber.org/zap"
)

func LoadPlugins(loader *plugins.PluginLoader, conf v1beta1.PluginsSpec) {
	for _, dir := range conf.Dirs {
		pluginPaths, err := plugin.Discover("plugin_*", dir)
		if err != nil {
			continue
		}
		for _, p := range pluginPaths {
			md, err := pluginmeta.ReadMetadata(p)
			if err != nil {
				loader.Logger.With(
					zap.String("plugin", p),
				).Error("failed to read plugin metadata", zap.Error(err))
				continue
			}
			cc := plugins.ClientConfig(md, plugins.ClientScheme)
			loader.Load(md, cc)
		}
	}
}
