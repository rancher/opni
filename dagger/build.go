package main

import (
	"fmt"
	"os"
	"path"
	"strings"

	"dagger.io/dagger"
)

type buildOpts struct {
	Path    string
	Output  string
	BuildID string
	Tags    []string
}

func buildMainPackage(ctr *dagger.Container, opts buildOpts) *dagger.Container {
	tag, _ := os.LookupEnv("BUILD_VERSION")
	tag = strings.TrimSpace(tag)

	version := "unversioned"
	if tag != "" {
		version = tag
	}

	args := []string{
		"go", "build", "-v",
		"-ldflags", fmt.Sprintf("-w -s -X github.com/rancher/opni/pkg/versions.Version=%s", version),
		"-trimpath",
		"-o", opts.Output,
	}
	if len(opts.Tags) > 0 {
		args = append(args, fmt.Sprintf("-tags=%s", strings.Join(opts.Tags, ",")))
	}

	// disable vcs stamping inside git worktrees if the linked git directory doesn't exist
	dotGit, err := os.Stat(".git")
	if err != nil || !dotGit.IsDir() {
		fmt.Println("disabling vcs stamping inside worktree")
		args = append(args, "-buildvcs=false")
	}

	args = append(args, opts.Path)

	return ctr.WithEnvVariable("CGO_ENABLED", "0").WithExec(args).WithExec([]string{"go", "tool", "buildid", opts.Output}, dagger.ContainerWithExecOpts{
		RedirectStdout: opts.Output + ".buildid",
	})
}

func (b *builder) buildPackages(ctr *dagger.Container) *dagger.Container {
	return ctr.
		WithEnvVariable("CGO_ENABLED", "0").
		WithExec([]string{"go", "build", "-v", "-buildmode=archive", "-trimpath", "-tags=noagentv1,nomsgpack", "./..."})
}

func (b *builder) build(mageBuild *dagger.Container, nodeBuild *dagger.Container) *dagger.Container {
	// optimization: (todo: improve further)
	// build the main opni binary first so that all shared packages are cached
	// when plugins and the minimal binary are built
	build := b.buildPackages(mageBuild)
	webDist := b.buildWeb(nodeBuild)
	b.buildOpni(build.WithMountedDirectory(path.Join(b.workdir, "web/dist"), webDist))
	b.buildOpniMinimal(build)
	b.buildPlugins(build)
	return build
}

func (b *builder) buildWeb(pipeline *dagger.Container) *dagger.Directory {
	return pipeline.
		WithExec([]string{"yarn", "install"}).
		WithExec([]string{"yarn", "build"}).
		Directory(path.Join(b.workdir, "web/dist"))
}

func (b *builder) buildOpni(pipeline *dagger.Container) *dagger.Container {
	// sources := b.client.Host().Directory(".", dagger.HostDirectoryOpts{
	// 	Include: []string{
	// 		"go.mod",
	// 		"go.sum",
	// 		"apis/",
	// 		"cmd/",
	// 		"controllers/",
	// 		"internal/",
	// 		"pkg/",
	// 		"web/",
	// 		"plugins/*/pkg/apis",
	// 	},
	// }).WithoutDirectory("internal/cmd")

	ctr := buildMainPackage(pipeline, buildOpts{
		Path:   "./cmd/opni",
		Output: "./bin/opni",
		Tags:   []string{"noagentv1", "nomsgpack"},
	})
	b.ExportToPath(ctr, "bin/opni")
	return ctr
}

func (b *builder) buildOpniMinimal(pipeline *dagger.Container) *dagger.Container {
	// sources := b.client.Host().Directory(".", dagger.HostDirectoryOpts{
	// 	Include: []string{
	// 		"go.mod",
	// 		"go.sum",
	// 		"apis/",
	// 		"cmd/",
	// 		"controllers/",
	// 		"internal/",
	// 		"pkg/",
	// 		"web/",
	// 	},
	// }).WithoutDirectory("internal/cmd")

	ctr := buildMainPackage(pipeline, buildOpts{
		Path:   "./cmd/opni",
		Output: "./bin/opni-minimal",
		Tags:   []string{"minimal", "noagentv1", "noscheme_thirdparty", "nomsgpack"},
	})
	b.ExportToPath(ctr, "bin/opni-minimal")
	return pipeline
}

func (b *builder) buildPlugins(pipeline *dagger.Container) *dagger.Container {
	// sources := b.client.Host().Directory(".", dagger.HostDirectoryOpts{
	// 	Include: []string{
	// 		"go.mod",
	// 		"go.sum",
	// 		"apis/",
	// 		"controllers/",
	// 		"internal/",
	// 		"pkg/",
	// 		"plugins/",
	// 	},
	// })
	// ctr := pipeline.WithMountedDirectory(b.workdir, sources)

	for _, entry := range b.hostInfo.PluginDirs {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// todo: how to make each plugin not depend on previous plugins in the list?
		b.buildPlugin("./"+path.Join("plugins", name), "./"+path.Join("bin/plugins/", "plugin_"+name))(pipeline)
	}
	return pipeline
}

func (b *builder) buildPlugin(path, output string) func(pipeline *dagger.Container) *dagger.Container {
	return func(pipeline *dagger.Container) *dagger.Container {
		c := buildMainPackage(pipeline, buildOpts{
			Path:   path,
			Output: output,
			Tags:   []string{"noagentv1", "nomsgpack"},
		})
		b.ExportToPath(c, output)
		return c
	}
}
