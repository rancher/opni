package targets

import (
	"context"
	"os"

	"github.com/magefile/mage/mg"
)

func (Build) Plugin(ctx context.Context, name string) error {
	_, tr := Tracer.Start(ctx, "target.build.plugin."+name)
	defer tr.End()

	// create bin/plugins if it doesn't exist
	if _, err := os.Stat("bin/plugins"); os.IsNotExist(err) {
		if err := os.MkdirAll("bin/plugins", 0755); err != nil {
			return err
		}
	}
	return buildMainPackage(buildOpts{
		Path:   "./plugins/" + name,
		Output: "bin/plugins/plugin_" + name,
		Tags:   []string{"nomsgpack"},
	})
}

func (Build) Plugins(ctx context.Context) {
	mg.CtxDeps(ctx, Build.Archives)

	ctx, tr := Tracer.Start(ctx, "target.build.plugins")
	defer tr.End()

	plugins := pluginList()
	deps := make([]any, 0, len(plugins))
	for _, plugin := range plugins {
		deps = append(deps, mg.F(Build.Plugin, plugin))
	}
	mg.CtxDeps(ctx, deps...)
}

func pluginList() []string {
	pluginDirs, err := os.ReadDir("plugins/")
	if err != nil {
		return nil
	}
	list := make([]string, 0, len(pluginDirs))
	for _, pluginDir := range pluginDirs {
		if !pluginDir.IsDir() {
			continue
		}
		list = append(list, pluginDir.Name())
	}
	return list
}
