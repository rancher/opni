package main

import (
	"context"
	encodingjson "encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"dagger.io/dagger"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"github.com/mitchellh/mapstructure"
	"github.com/rancher/opni/dagger/config"
	"github.com/rancher/opni/dagger/helm"
	"github.com/rancher/opni/dagger/x/cmds"
	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"
)

type Builder struct {
	config.BuilderConfig
	ctx       context.Context
	caches    config.Caches
	cacheMode string
	client    *dagger.Client
	sources   *dagger.Directory
	workdir   string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

type runOptions struct {
	Context context.Context
	Client  *dagger.Client
	Args    []string
}

func run(opts ...runOptions) error {
	if len(opts) == 0 {
		opts = append(opts, runOptions{
			Args: os.Args,
		})
	}
	var debug bool
	var cacheMode string
	var configs []string
	var showConfig bool
	var outputFormat string
	pf := pflag.NewFlagSet("dagger", pflag.ExitOnError)
	pf.BoolVar(&debug, "debug", false, "Enable debug logging")
	pf.StringVar(&cacheMode, "cache-mode", "volumes", "Cache mode (volumes|none)")
	pf.StringSliceVarP(&configs, "config", "c", nil, "Path to one or more config files")
	pf.BoolVar(&showConfig, "show-config", false, "Print the final config and exit")
	pf.StringVarP(&outputFormat, "output-format", "o", "table", "Output format used when --show-config is set (table|json|yaml|toml)")
	configFlagSet := config.BuildFlagSet(reflect.TypeOf(config.BuilderConfig{}))
	pf.SortFlags = false
	pf.AddFlagSet(configFlagSet)
	if err := pf.Parse(opts[0].Args); err != nil {
		return err
	}

	k := koanf.NewWithConf(koanf.Conf{
		Delim:       ".",
		StrictMerge: true,
	})

	// Load Defaults
	must(k.Load(structs.Provider(config.BuilderConfig{}, "koanf"), nil))

	// Load from config file
	for _, conf := range configs {
		if err := k.Load(config.AutoLoader(conf)); err != nil {
			return err
		}
		if debug {
			fmt.Printf("[config] loading %s\n", conf)
		}
	}

	var ctx context.Context
	var client *dagger.Client
	if opts[0].Context != nil {
		ctx = opts[0].Context
	} else {
		ctx = context.Background()
	}
	if opts[0].Client != nil {
		client = opts[0].Client
	} else {
		var err error
		client, err = dagger.Connect(ctx, dagger.WithLogOutput(os.Stdout))
		if err != nil {
			return err
		}
	}

	// Load from environment

	// First load from some known environment variables as defaults
	for _, sc := range config.SpecialCaseEnvVars(client) {
		str, ok := os.LookupEnv(sc.EnvVar)
		if !ok {
			continue
		}
		for _, key := range sc.Keys {
			val := sc.Converter(key, str)
			switch val.(type) {
			case string:
				if debug {
					fmt.Printf("[config] setting %s=%s from env %s\n", key, val, sc.EnvVar)
				}
			case *dagger.Secret:
				if debug {
					fmt.Printf("[config] setting %s=<secret> from env %s\n", key, sc.EnvVar)
				}
			}
			k.Set(key, val)
		}
	}

	// Then load from standard environment variables (these take priority)
	must(k.Load(env.ProviderWithValue(config.EnvPrefix, ".", func(envvar string, val string) (string, any) {
		key := strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(envvar, config.EnvPrefix)), "_", ".")
		if strings.Contains(key, "secret") {
			if debug {
				fmt.Printf("[config] setting %s=<secret> from env %s\n", key, envvar)
			}
			return key, client.SetSecret(key, val)
		}
		if debug {
			fmt.Printf("[config] setting %s=%s from env %s\n", key, val, envvar)
		}
		return key, val
	}), nil))

	must(k.Load(posflag.Provider(configFlagSet, ".", k), nil))

	var builderConfig config.BuilderConfig
	if err := k.UnmarshalWithConf("", nil, koanf.UnmarshalConf{
		DecoderConfig: &mapstructure.DecoderConfig{
			WeaklyTypedInput: false,
			ErrorUnused:      true,
			Result:           &builderConfig,
		},
	}); err != nil {
		fmt.Println(string(config.Marshal(k, outputFormat)))
		return err
	}

	if err := config.Validate(&builderConfig); err != nil {
		msg := err.Error()
		msg = strings.ReplaceAll(msg, `BuilderConfig.`, "")
		fmt.Fprintln(os.Stderr, msg)
		fmt.Println(string(config.Marshal(k, outputFormat)))
		return err
	}

	if showConfig {
		fmt.Println(string(config.Marshal(k, outputFormat)))
		return nil
	}

	builder := &Builder{
		BuilderConfig: builderConfig,
		ctx:           ctx,
		client:        client,
		caches:        config.SetupCaches(client, cacheMode),
		workdir:       "/src",
		sources: client.Host().Directory(".", dagger.HostDirectoryOpts{
			Include: []string{
				"go.mod",
				"go.sum",
				"aiops/",
				"apis/",
				"cmd/",
				"config/",
				"controllers/",
				"images/",
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
				"test/",
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

	err := builder.run(ctx)
	client.Close()
	if err != nil {
		return err
	}
	return nil
}

func (b *Builder) run(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return b.runInTreeBuilds(ctx)
	})
	eg.Go(func() error {
		return b.runOutOfTreeBuilds(ctx)
	})
	return eg.Wait()
}

func (b *Builder) runInTreeBuilds(ctx context.Context) error {
	goBase := b.goBase()
	nodeBase := b.nodeBase()
	alpineBase := b.alpineBase()

	goBuild := goBase.
		Pipeline("Go Build").
		WithDirectory(b.workdir, b.sources, dagger.ContainerWithDirectoryOpts{
			Include: []string{"go.mod", "go.sum", "tools.go"},
		}).
		WithExec([]string{"go", "mod", "download"}).
		WithEnvVariable("CGO_ENABLED", "1").
		WithExec([]string{"sh", "-c", `go install $(go list -f '{{join .Imports " "}}' tools.go)`}).
		WithEnvVariable("CGO_ENABLED", "0"). // important for cached magefiles
		WithExec([]string{"go", "install", "github.com/magefile/mage@latest"}).
		WithDirectory(b.workdir, b.sources)

	nodeBuild := nodeBase.
		Pipeline("Node Build").
		WithDirectory(filepath.Join(b.workdir, "web"), b.sources.Directory("web"), dagger.ContainerWithDirectoryOpts{
			Include: []string{"package.json", "yarn.lock"},
		}).
		WithExec(yarn([]string{"install", "--frozen-lockfile"})).
		WithDirectory(filepath.Join(b.workdir, "web"), b.sources.Directory("web")).
		With(b.caches.NodeModules).
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
	lint := goBuild.
		Pipeline("Run Linter").
		WithMountedDirectory(b.workdir, b.sources).
		WithMountedFile(linterPluginPath, linterPlugin.File(linterPluginPath)).
		WithExec([]string{"golangci-lint", "run", "-v", "--fast"})

	test := opni.
		WithExec(mage("test:binconfig"))

	{ // lint & test
		var eg errgroup.Group
		if b.Lint {
			eg.Go(func() error {
				if _, err := lint.Sync(ctx); err != nil {
					return err
				}
				return nil
			})
		}

		if b.Test {
			eg.Go(func() error {
				var opts cmds.TestBinOptions
				confJson, err := test.Stdout(ctx)
				if err != nil {
					return err
				}
				if err := encodingjson.Unmarshal([]byte(confJson), &opts); err != nil {
					return err
				}
				test = cmds.TestBin(b.client, test, opts)
				if b.Coverage.Export {
					_, err := test.Pipeline("Run Tests").
						WithExec(mage("test")).
						File(filepath.Join(b.workdir, "cover.out")).
						Export(ctx, "cover.out")
					return err
				}
				_, err = test.Pipeline("Run Tests").
					WithEnvVariable("DISABLE_COVERAGE", "1").
					WithExec(mage("test")).
					Sync(ctx)
				return err
			})
		}

		if err := eg.Wait(); err != nil {
			return err
		}
	}

	fullImage := alpineBase.
		Pipeline("Full Image").
		WithFile("/usr/bin/opni", opni.File(b.bin("opni"))).
		WithDirectory("/var/lib/opni/plugins", plugins.Directory(b.bin("plugins")))

	minimalImage := alpineBase.
		Pipeline("Minimal Image").
		WithFile("/usr/bin/opni", minimal.File(b.bin("opni-minimal")))

	charts := goBuild.
		Pipeline("Charts").
		WithFile(b.ciTarget("charts")).
		WithExec(mage("charts"))

	// export and push artifacts

	var eg errgroup.Group

	if b.Charts.Git.Export {
		eg.Go(func() error {
			charts.Directory(filepath.Join(b.workdir, "charts")).Export(ctx, "./charts")
			charts.Directory(filepath.Join(b.workdir, "assets")).Export(ctx, "./assets")
			return nil
		})
	}

	if b.Charts.Git.Push {
		eg.Go(func() error {
			return helm.PublishToChartsRepo(ctx, b.client, helm.PublishOpts{
				Target:         b.Charts.Git,
				BuildContainer: charts,
				Caches:         b.caches,
			})
		})
	}

	if b.Charts.OCI.Push {
		eg.Go(func() error {
			return helm.Push(ctx, b.client, helm.PushOpts{
				Target: b.Charts.OCI,
				Dir:    charts.Directory(filepath.Join(b.workdir, "assets")),
			})
		})
	}

	eg.Go(func() error {
		var minimalRef string
		if b.Images.OpniMinimal.Push {
			var err error
			minimalRef, err = minimalImage.
				WithRegistryAuth(b.Images.OpniMinimal.RegistryAuth()).
				Publish(ctx, b.Images.OpniMinimal.Ref())
			if err != nil {
				return fmt.Errorf("failed to publish image %s: %w", b.Images.OpniMinimal.Ref(), err)
			}
			fmt.Println("published image:", minimalRef)
		}

		if b.Images.Opni.Push {
			if minimalRef != "" {
				fullImage = fullImage.
					WithEnvVariable("OPNI_MINIMAL_IMAGE_REF", minimalRef).
					WithLabel("opni.io.minimal-image-ref", minimalRef)
			}
			ref, err := fullImage.
				WithRegistryAuth(b.Images.Opni.RegistryAuth()).
				Publish(ctx, b.Images.Opni.Ref())
			if err != nil {
				return fmt.Errorf("failed to publish image %s: %w", b.Images.Opni.Ref(), err)
			}
			fmt.Println("published image:", ref)
		}

		return nil
	})

	return eg.Wait()
}

func (b *Builder) runOutOfTreeBuilds(ctx context.Context) error {
	opensearchDashboards := b.BuildOpensearchDashboardsImage()
	opensearch := b.BuildOpensearchImage()
	var pythonBase *dagger.Container
	if b.Images.PythonBase.Push {
		pythonBase = b.BuildOpniPythonBase()
	} else {
		pythonBase = b.client.Container().From(b.Images.PythonBase.Ref())
	}
	updateSvc := b.BuildOpensearchUpdateServiceImage(pythonBase)

	if b.Images.Opensearch.Dashboards.Push {
		ref, err := opensearchDashboards.
			WithRegistryAuth(b.Images.Opensearch.Dashboards.RegistryAuth()).
			Publish(ctx, b.Images.Opensearch.Dashboards.Ref())
		if err != nil {
			return fmt.Errorf("failed to publish image %s: %w", b.Images.Opensearch.Dashboards.Ref(), err)
		}
		fmt.Println("published image:", ref)
	}
	if b.Images.Opensearch.Opensearch.Push {
		ref, err := opensearch.
			WithRegistryAuth(b.Images.Opensearch.Opensearch.RegistryAuth()).
			Publish(ctx, b.Images.Opensearch.Opensearch.Ref())
		if err != nil {
			return fmt.Errorf("failed to publish image %s: %w", b.Images.Opensearch.Opensearch.Ref(), err)
		}
		fmt.Println("published image:", ref)
	}
	if b.Images.PythonBase.Push {
		ref, err := pythonBase.
			WithRegistryAuth(b.Images.PythonBase.RegistryAuth()).
			Publish(ctx, b.Images.PythonBase.Ref())
		if err != nil {
			return fmt.Errorf("failed to publish image %s: %w", b.Images.PythonBase.Ref(), err)
		}
		fmt.Println("published image:", ref)
	}
	if b.Images.Opensearch.UpdateService.Push {
		ref, err := updateSvc.
			WithRegistryAuth(b.Images.Opensearch.UpdateService.RegistryAuth()).
			Publish(ctx, b.Images.Opensearch.UpdateService.Ref())
		if err != nil {
			return fmt.Errorf("failed to publish image %s: %w", b.Images.Opensearch.UpdateService.Ref(), err)
		}
		fmt.Println("published image:", ref)
	}

	return nil
}
