package main

import (
	"os"
	"path/filepath"

	//mage:import
	"github.com/rancher/opni/magefiles/targets"
)

func init() {
	if target, ok := os.LookupEnv("MAGE_SYMLINK_CACHED_BINARY"); ok {
		x, err := os.Executable()
		if err != nil {
			panic(err)
		}
		basename := filepath.Base(x)
		if basename == target {
			return
		}
		dir := filepath.Dir(x)
		target := filepath.Join(dir, target)
		os.Remove(target)
		if err := os.Symlink(x, target); err != nil {
			panic(err)
		}
	}
}

var Default = targets.All

var Aliases = map[string]any{
	"test":     targets.Test.All,
	"build":    targets.Build.All,
	"generate": targets.Generate.All,
}
