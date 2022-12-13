package plugins

import (
	"github.com/hashicorp/go-plugin"
	"github.com/rancher/opni/pkg/plugins/meta"
	"go.uber.org/zap"
)

type Filter = func(meta.PluginMeta) bool

type DiscoveryConfig struct {
	Dir        string
	Filters    []Filter
	Logger     *zap.SugaredLogger
	QueryModes bool
}

func (dc DiscoveryConfig) Discover() []meta.PluginMeta {
	paths, err := plugin.Discover(DefaultPluginGlob, dc.Dir)
	if err != nil {
		panic(err)
	}
	var result []meta.PluginMeta
PLUGINS:
	for _, path := range paths {
		md, err := meta.ReadPath(path)
		if err != nil {
			if dc.Logger != nil {
				dc.Logger.With(
					zap.Error(err),
					"plugin", path,
				).Error("failed to read plugin metadata")
			}
			continue
		}
		if dc.QueryModes {
			modes, err := meta.QueryPluginModes(md.BinaryPath)
			if err != nil {
				if dc.Logger != nil {
					dc.Logger.With(
						zap.Error(err),
						"plugin", path,
					).Error("failed to query plugin modes")
				}
				continue
			}
			if md.ExtendedMetadata == nil {
				md.ExtendedMetadata = &meta.ExtendedPluginMeta{}
			}
			md.ExtendedMetadata.ModeList = modes
		}

		for i, filter := range dc.Filters {
			if !filter(md) {
				if dc.Logger != nil {
					dc.Logger.With(
						"plugin", path,
						"filter", i,
					).Debug("plugin ignored due to filter")
				}
				continue PLUGINS
			}
		}
		result = append(result, md)
	}
	return result
}
