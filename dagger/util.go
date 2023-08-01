package main

import (
	"fmt"
	"os"
	"path/filepath"

	"dagger.io/dagger"
	"github.com/rancher/opni/dagger/images"
)

func (b *Builder) bin(paths ...string) string {
	return filepath.Join(append([]string{b.workdir, "bin"}, paths...)...)
}

func (b *Builder) nodeBase() *dagger.Container {
	return images.NodeBase(b.client).
		With(b.caches.Yarn).
		WithEnvVariable("YARN_CACHE_FOLDER", "/cache/yarn").
		With(b.caches.NodeModules).
		WithEnvVariable("NODE_PATH", "/cache/node_modules").
		WithWorkdir(filepath.Join(b.workdir, "web"))
}

func (b *Builder) goBase() *dagger.Container {
	return images.GoBase(b.client).
		With(b.caches.Mage).
		With(b.caches.GoBuild).
		With(b.caches.GoMod).
		With(b.caches.GoBin).
		WithWorkdir(b.workdir)
}

func (b *Builder) alpineBase() *dagger.Container {
	return images.AlpineBase(b.client, images.WithPackages(
		"tzdata",
		"bash",
		"bash-completion",
		"curl",
		"tini",
	))
}

func (b *Builder) ciTarget(name string) (string, *dagger.File) {
	return filepath.Join(b.workdir, "magefiles", name+".go"),
		b.sources.File(filepath.Join("magefiles/ci", name+".go"))
}

func mage(target string, opts ...dagger.ContainerWithExecOpts) ([]string, dagger.ContainerWithExecOpts) {
	if len(opts) == 0 {
		opts = append(opts, dagger.ContainerWithExecOpts{})
	}
	return []string{"mage", "-v", target}, opts[0]
}

func yarn[S string | []string](target S, opts ...dagger.ContainerWithExecOpts) ([]string, dagger.ContainerWithExecOpts) {
	if len(opts) == 0 {
		opts = append(opts, dagger.ContainerWithExecOpts{})
	}
	var args []string
	switch target := any(target).(type) {
	case string:
		args = []string{"yarn", target}
	case []string:
		args = append([]string{"yarn"}, target...)
	}
	return args, opts[0]
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
