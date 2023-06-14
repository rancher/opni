package config

import (
	"os"

	"dagger.io/dagger"
)

const (
	CacheModeVolumes = "volumes"
	CacheModeNone    = "none"
)

type CacheVolume struct {
	*dagger.CacheVolume
	Path string
}

type Caches struct {
	GoMod       func(*dagger.Container) *dagger.Container
	GoBuild     func(*dagger.Container) *dagger.Container
	GoBin       func(*dagger.Container) *dagger.Container
	Mage        func(*dagger.Container) *dagger.Container
	Yarn        func(*dagger.Container) *dagger.Container
	NodeModules func(*dagger.Container) *dagger.Container
	TestBin     func(*dagger.Container) *dagger.Container
}

func SetupCaches(client *dagger.Client, cacheMode string) Caches {
	if _, ok := os.LookupEnv("CI"); ok {
		cacheMode = CacheModeNone
	}
	identity := func(ctr *dagger.Container) *dagger.Container { return ctr }
	switch cacheMode {
	case CacheModeVolumes:
		return Caches{
			GoMod: func(ctr *dagger.Container) *dagger.Container {
				return ctr.WithMountedCache("/go/pkg/mod", client.CacheVolume("gomod"))
			},
			GoBuild: func(ctr *dagger.Container) *dagger.Container {
				return ctr.WithMountedCache("/root/.cache/go-build", client.CacheVolume("gobuild"))
			},
			GoBin: func(ctr *dagger.Container) *dagger.Container {
				return ctr.WithMountedCache("/go/bin", client.CacheVolume("gobin"))
			},
			Mage: func(ctr *dagger.Container) *dagger.Container {
				return ctr.WithMountedCache("/root/.magefile", client.CacheVolume("mage"))
			},
			Yarn: func(ctr *dagger.Container) *dagger.Container {
				return ctr.WithMountedCache("/cache/yarn", client.CacheVolume("yarn"))
			},
			NodeModules: func(ctr *dagger.Container) *dagger.Container {
				return ctr.WithMountedCache("/src/web/node_modules", client.CacheVolume("node_modules"))
			},
		}
	case CacheModeNone:
		fallthrough
	default:
		return Caches{
			GoMod:       identity,
			GoBuild:     identity,
			GoBin:       identity,
			Mage:        identity,
			Yarn:        identity,
			NodeModules: identity,
		}
	}
}
