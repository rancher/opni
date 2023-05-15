package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"dagger.io/dagger"
	"github.com/rancher/opni/dagger/config"
	"github.com/rancher/opni/dagger/helm"
	"github.com/rancher/opni/dagger/images"
	"golang.org/x/sync/errgroup"
)

type Builder struct {
	config.BuilderConfig
	ctx     context.Context
	caches  config.Caches
	client  *dagger.Client
	sources *dagger.Directory
	workdir string
}

func main() {
	conf := config.BuilderConfig{
		Opni: config.ImageTarget{
			Repo: "docker.io/rancher/opni",
			Tag:  "latest",
		},
		OpniMinimal: config.ImageTarget{
			Repo:      "docker.io/rancher/opni-minimal",
			TagSuffix: "-minimal",
		},
		HelmOCI: config.ImageTarget{
			Repo: "ghcr.io/rancher",
		},
	}
	fs := conf.FlagSet()
	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if conf.Opni.Tag != "" && conf.OpniMinimal.Tag == "" {
		conf.OpniMinimal.Tag = conf.Opni.Tag
	}

	ctx := context.Background()
	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stdout))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	conf.LoadSecretsFromEnv(client)
	builder := &Builder{
		BuilderConfig: conf,
		ctx:           ctx,
		client:        client,

		workdir: "/src",
		sources: client.Host().Directory(".", dagger.HostDirectoryOpts{
			Include: []string{
				"go.mod",
				"go.sum",
				"aiops/",
				"apis/",
				"cmd/",
				"config/",
				"controllers/",
				"internal/alerting/",
				"internal/bench/",
				"internal/codegen/",
				"internal/cortex/",
				"internal/linter/*.go",
				"magefiles/",
				"packages/",
				"pkg/",
				"plugins/",
				"web/",
				"configuration.yaml",
				".golangci.yaml",
				"tools.go",
				"LICENSE",
			},
			Exclude: []string{
				"magefiles/trace",
				"web/dist/",
				"web/node_modules/",
			},
		}).WithNewFile("web/dist/.gitkeep", ""),
	}
	builder.SetupCaches()

	err = builder.run(ctx)
	client.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (b *Builder) run(ctx context.Context) error {
	goBase := b.goBase()
	nodeBase := b.nodeBase()
	alpineBase := b.alpineBase()

	goBuild := goBase.
		Pipeline("Go Build").
		WithMountedDirectory(b.workdir, b.sources).
		WithExec([]string{"sh", "-c", `go install $(go list -f '{{join .Imports " "}}' tools.go)`})

	nodeBuild := nodeBase.
		Pipeline("Node Build").
		WithMountedDirectory(filepath.Join(b.workdir, "web"), b.sources.Directory("web")).
		WithMountedCache(b.caches.NodeModules()).
		WithExec(yarn([]string{"install", "--frozen-lockfile"})).
		WithExec(yarn("build"))

	generated := goBuild.
		Pipeline("Generate").
		WithExec(mage("generate:all"))

	archives := generated.
		Pipeline("Build Archives").
		WithExec(mage("build:archives"))

	plugins := archives.
		Pipeline("Build Plugins").
		WithExec(mage("build:plugins"))

	webDist := filepath.Join(b.workdir, "web", "dist")
	opni := archives.
		Pipeline("Build Opni").
		WithMountedDirectory(webDist, nodeBuild.Directory(webDist)).
		WithExec(mage("build:opni"))

	minimal := archives.
		Pipeline("Build Opni Minimal").
		WithExec(mage("build:opniminimal"))

	linterPlugin := goBuild.
		Pipeline("Build Linter Plugin").
		WithExec(mage("build:linter"))

	linterPluginPath := filepath.Join(b.workdir, "internal/linter/linter.so")
	lint := goBase.
		Pipeline("Lint").
		WithMountedDirectory(b.workdir, b.sources).
		WithMountedFile(linterPluginPath, linterPlugin.File(linterPluginPath)).
		WithExec([]string{"golangci-lint", "run"})

	fullImage := alpineBase.
		Pipeline("Full Image").
		WithFile("/usr/bin/opni", opni.File(b.bin("opni"))).
		WithDirectory("/var/lib/opni/plugins", plugins.Directory(b.bin("plugins")))

	minimalImage := alpineBase.
		Pipeline("Minimal Image").
		WithFile("/usr/bin/opni", minimal.File(b.bin("opni-minimal")))

	if b.HelmOCI.Enabled || b.Helm.Enabled {
		charts := goBuild.
			Pipeline("Charts").
			WithFile(b.ciTarget("charts")).
			WithEnvVariable("MAGE_SYMLINK_CACHED_BINARY", "1").
			WithExec(mage("charts"))

		if b.Export {
			charts.Directory(filepath.Join(b.workdir, "charts")).Export(ctx, "./charts")
			charts.Directory(filepath.Join(b.workdir, "assets")).Export(ctx, "./assets")
		}

		if b.Push {
			if b.HelmOCI.Enabled {
				if err := helm.Push(ctx, b.client, helm.PushOptions{
					Target: b.HelmOCI,
					Dir:    charts.Directory(filepath.Join(b.workdir, "assets")),
					Charts: []string{"opni/opni-0.10.0-rc4.tgz"},
				}); err != nil {
					return fmt.Errorf("pushing helm chart images: %w", err)
				}
			}
			if b.Helm.Enabled {
				if err := helm.PublishToChartsRepo(ctx, b.client, helm.PublishOptions{
					Target:         b.Helm,
					BuildContainer: charts,
					Caches:         b.caches,
				}); err != nil {
					return fmt.Errorf("publishing helm charts: %w", err)
				}
			}
		}
	}

	{
		eg, ctx := errgroup.WithContext(ctx)
		if b.Lint {
			sync(ctx, eg, lint)
		}
		if !b.Push {
			if b.Opni.Enabled {
				sync(ctx, eg, fullImage)
			}
			if b.OpniMinimal.Enabled {
				sync(ctx, eg, minimalImage)
			}
		}
		if err := eg.Wait(); err != nil {
			return err
		}
	}

	if b.Push {
		var minimalRef string
		if b.OpniMinimal.Enabled {
			var err error
			minimalRef, err = minimalImage.Publish(ctx, fmt.Sprintf("%s:%s%s", b.OpniMinimal.Repo, b.OpniMinimal.Tag, b.OpniMinimal.TagSuffix))
			if err != nil {
				return fmt.Errorf("failed to publish image: %w", err)
			}
			fmt.Println("published image:", minimalRef)
		}

		if b.Opni.Enabled {
			ref, err := fullImage.
				WithEnvVariable("OPNI_MINIMAL_IMAGE_REF", minimalRef).
				WithLabel("opni.io.minimal-image-ref", minimalRef).
				Publish(ctx, fmt.Sprintf("%s:%s%s", b.Opni.Repo, b.Opni.Tag, b.Opni.TagSuffix))
			if err != nil {
				return fmt.Errorf("failed to publish image: %w", err)
			}
			fmt.Println("published image:", ref)
		}
		return nil
	}

	return nil
}

func (b *Builder) bin(paths ...string) string {
	return filepath.Join(append([]string{b.workdir, "bin"}, paths...)...)
}

func (b *Builder) nodeBase() *dagger.Container {
	return images.NodeBase(b.client).
		WithMountedCache(b.caches.Yarn()).
		WithEnvVariable("YARN_CACHE_FOLDER", "/cache/yarn").
		WithWorkdir(filepath.Join(b.workdir, "web"))
}

func (b *Builder) goBase() *dagger.Container {
	return images.GoBase(b.client).
		WithMountedCache(b.caches.Mage()).
		WithMountedCache(b.caches.GoBuild()).
		WithMountedCache(b.caches.GoMod()).
		WithMountedCache(b.caches.GoBin()).
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
	return filepath.Join(b.workdir, "magefiles/targets", name+".go"),
		b.sources.File(filepath.Join("magefiles/targets/ci", name+".go"))
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

func sync(ctx context.Context, eg *errgroup.Group, pipelines ...*dagger.Container) {
	for _, pipeline := range pipelines {
		pipeline := pipeline
		eg.Go(func() error {
			if _, err := pipeline.Sync(ctx); err != nil {
				return err
			}
			return nil
		})
	}
}

func (b *Builder) SetupCaches() {
	b.caches = config.Caches{
		GoMod: func() (string, *dagger.CacheVolume) {
			return "/go/pkg/mod", b.client.CacheVolume("gomod")
		},
		GoBuild: func() (string, *dagger.CacheVolume) {
			return "/root/.cache/go-build", b.client.CacheVolume("gobuild")
		},
		GoBin: func() (string, *dagger.CacheVolume) {
			return "/go/bin", b.client.CacheVolume("gobin")
		},
		Mage: func() (string, *dagger.CacheVolume) {
			return "/root/.magefile", b.client.CacheVolume("mage")
		},
		Yarn: func() (string, *dagger.CacheVolume) {
			return "/cache/yarn", b.client.CacheVolume("yarn")
		},
		NodeModules: func() (string, *dagger.CacheVolume) {
			return filepath.Join(b.workdir, "web/node_modules"), b.client.CacheVolume("node_modules")
		},
	}
}
