package main

import (
	//mage:import
	_ "github.com/rancher/opni/magefiles/targets"

	"os"
	"path/filepath"
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
