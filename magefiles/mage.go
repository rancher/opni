package main

import (
	//mage:import
	_ "github.com/rancher/opni/magefiles/targets"

	"os"
	"path/filepath"
)

func init() {
	if _, ok := os.LookupEnv("MAGE_SYMLINK_CACHED_BINARY"); ok {
		name, err := os.Executable()
		if err != nil {
			panic(err)
		}
		basename := filepath.Base(name)
		if basename == "latest" {
			return
		}
		dir := filepath.Dir(name)
		target := filepath.Join(dir, "latest")
		os.Remove(target)
		if err := os.Link(name, target); err != nil {
			panic(err)
		}
	}
}
