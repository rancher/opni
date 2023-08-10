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

func (b *Builder) base() *dagger.Container {
	return images.Base(b.client).
		With(b.caches.Mage).
		With(b.caches.GoBuild).
		With(b.caches.GoMod).
		With(b.caches.GoBin).
		With(b.caches.Yarn).
		WithEnvVariable("YARN_CACHE_FOLDER", "/cache/yarn").
		With(b.caches.NodeModules).
		WithEnvVariable("NODE_PATH", "/cache/node_modules").
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

func installTools(ctr *dagger.Container) *dagger.Container {
	for _, tool := range []string{
		"go.uber.org/mock/mockgen@latest",
		"sigs.k8s.io/controller-tools/cmd/controller-gen@latest",
		"sigs.k8s.io/kustomize/kustomize/v5@latest",
	} {
		ctr = ctr.WithExec([]string{"go", "install", "-v", "-x", tool})
	}
	return ctr
}
