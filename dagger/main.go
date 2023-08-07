package main

import (
	"context"
	"encoding/json"
	"errors"
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
		if errors.Is(err, pflag.ErrHelp) {
			return
		}
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
	var setup bool
	var cacheMode string
	var configs []string
	var showConfig bool
	var outputFormat string
	pf := pflag.NewFlagSet("dagger", pflag.ContinueOnError)
	pf.BoolVar(&debug, "debug", false, "Enable debug logging")
	pf.StringVar(&cacheMode, "cache-mode", "volumes", "Cache mode (volumes|none)")
	pf.StringSliceVarP(&configs, "config", "c", nil, "Path to one or more config files")
	pf.BoolVar(&setup, "setup", false, "Interactive configuration setup")
	pf.BoolVar(&showConfig, "show-config", false, "Print the final config and exit")
	pf.StringVarP(&outputFormat, "output-format", "o", "table", "Output format used when --show-config is set (table|json|yaml|toml)")
	configFlagSet := config.BuildFlagSet(reflect.TypeOf(config.BuilderConfig{}))
	pf.SortFlags = false
	pf.Usage = func() {
		pf.PrintDefaults()
		fmt.Printf(`
To create a new config file interactively, use --setup, which will guide you through setting up
a new config file with common default options. This file can then be used with --config.

To see the full list of available flags, use --show-config. Any of the keys shown in the table can
be used verbatim as a flag. For example:
- A key 'foo.bar' of type string can be set via '--foo.bar=baz'.
- A key 'foo.bar' of type bool can be set via '--foo.bar' (assumed true), or '--foo.bar={true|false}'.
- A key 'foo.bar' of type []string can be set via '--foo.bar=a,b,c'.

Config files are loaded in order, and fields set in earlier files can be overridden by the same
fields set in later files. Environment variables take precedence over config files, and flags
take precedence over environment variables. Values are always replaced, not merged, when
overriding fields.

Secrets can be configured via flags, but it is recommended to use environment variables instead,
as flags are visible in the process list and in the shell history. The corresponding environment
variables for each flag are shown in the table.

Some keys have alternate environment variables that can be used to bulk-assign related
fields, most commonly usernames, passwords and image tags.
The available alternate environment variables and associated keys are as follows:
%s
The following validation rules are applied after loading all files, environment variables, and flags:
%s
`, config.SpecialCaseEnvVarHelp(), config.ValidationHelp())
	}
	pf.AddFlagSet(configFlagSet)
	if err := pf.Parse(opts[0].Args); err != nil {

		return err
	}

	if setup {
		// check if we're running inside dagger
		if _, ok := os.LookupEnv("DAGGER_SESSION_PORT"); ok {
			fmt.Println(`Cannot run interactive setup inside dagger; use 'go run ./dagger --setup' instead`)
			os.Exit(1)
		}
		config.RunSetup()
		return nil
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
				"internal/linter/",
				"internal/cmd/lint/",
				"magefiles/",
				"packages/",
				"pkg/",
				"plugins/",
				"web/",
				"test/",
				"configuration.yaml",
				".golangci.yaml",
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
		With(installTools).
		WithEnvVariable("CGO_ENABLED", "0").
		WithEnvVariable("GOBIN", "/usr/bin"). // important for cached mage binary
		WithExec([]string{"go", "install", "github.com/magefile/mage@latest"}).
		WithoutEnvVariable("GOBIN").
		WithDirectory(b.workdir, b.sources)

	nodeBuild := nodeBase.
		Pipeline("Node Build").
		WithDirectory(filepath.Join(b.workdir, "web"), b.sources.Directory("web")).
		WithExec([]string{"ln", "-s", "/cache/node_modules", filepath.Join(b.workdir, "web", "node_modules")}).
		WithExec([]string{"ls", "-halL", "node_modules"}).
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

	lint := archives.
		Pipeline("Run Linter").
		With(b.caches.GolangciLint).
		WithExec(mage("lint"))

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
			chromedp := b.client.Container().
				From("chromedp/headless-shell:114.0.5735.199")

			eg.Go(func() error {
				var opts cmds.TestBinOptions
				confJson, err := test.Stdout(ctx)
				if err != nil {
					return err
				}
				if err := json.Unmarshal([]byte(confJson), &opts); err != nil {
					return err
				}
				test = cmds.TestBin(b.client, test, opts).
					WithMountedDirectory("/headless-shell", chromedp.Directory("/headless-shell")).
					WithEnvVariable("PATH", "$PATH:/headless-shell", dagger.ContainerWithEnvVariableOpts{Expand: true})

				if b.Coverage.Export {
					_, err := test.Pipeline("Run Tests").
						WithExec(mage("test:cover")).
						File(filepath.Join(b.workdir, "cover.out")).
						Export(ctx, "cover.out")
					return err
				}
				_, err = test.Pipeline("Run Tests").
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
		WithDirectory("/var/lib/opni/plugins", plugins.Directory(b.bin("plugins"))).
		WithEntrypoint([]string{"opni"})

	minimalImage := alpineBase.
		Pipeline("Minimal Image").
		WithFile("/usr/bin/opni", minimal.File(b.bin("opni-minimal"))).
		WithEntrypoint([]string{"opni"})

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
			charts.File(filepath.Join(b.workdir, "index.yaml")).Export(ctx, "./index.yaml")
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
	opensearchDashboards := b.client.Container().
		Pipeline("Opensearch Dashboards Image").
		From(fmt.Sprintf("opensearchproject/opensearch-dashboards:%s", b.Images.Opensearch.Build.DashboardsVersion)).
		WithExec([]string{"opensearch-dashboards-plugin", "install",
			fmt.Sprintf("https://github.com/rancher/opni-ui/releases/download/plugin-%[1]s/opni-dashboards-plugin-%[1]s.zip", b.Images.Opensearch.Build.PluginVersion),
		})

	opensearch := b.client.Container().
		Pipeline("Opensearch Image").
		From(fmt.Sprintf("opensearchproject/opensearch:%s", b.Images.Opensearch.Build.OpensearchVersion)).
		WithExec([]string{"opensearch-plugin", "-s", "install", "-b",
			fmt.Sprintf("https://github.com/rancher/opni-ingest-plugin/releases/download/v%s/opnipreprocessing.zip", b.Images.Opensearch.Build.PluginVersion),
		}).
		WithDirectory("/usr/share/opensearch/extensions", b.client.Directory(), dagger.ContainerWithDirectoryOpts{Owner: "1000:1000"})

	pythonBase := b.client.Container().
		Pipeline("Opni Python Base Image").
		From("registry.suse.com/suse/sle15:15.3").
		WithExec([]string{"zypper", "--non-interactive", "in", "python39", "python39-pip", "python39-devel"})

	baseBuilder := pythonBase.
		WithExec([]string{"zypper", "--non-interactive", "in", "gcc"}).
		WithExec([]string{"python3.9", "-m", "venv", "/opt/venv"}).
		WithFile("/requirements.txt", b.sources.File("images/python/requirements.txt")).
		WithExec([]string{"/opt/venv/bin/pip", "install", "-r", "/requirements.txt"})

	torchBuilder := baseBuilder.
		WithFile("/requirements-torch.txt", b.sources.File("images/python/requirements-torch.txt")).
		WithExec([]string{"/opt/venv/bin/pip", "install", "-r", "/requirements-torch.txt"})

	opniPythonBase := pythonBase.
		WithDirectory("/opt/venv", baseBuilder.Directory("/opt/venv")).
		WithEnvVariable("PATH", "/opt/venv/bin:${PATH}", dagger.ContainerWithEnvVariableOpts{Expand: true})

	opniPythonTorch := opniPythonBase.
		WithDirectory("/opt/venv", torchBuilder.Directory("/opt/venv")).
		WithEnvVariable("PATH", "/usr/local/nvidia/bin:/usr/local/cuda/bin:${PATH}", dagger.ContainerWithEnvVariableOpts{Expand: true}).
		WithEnvVariable("LD_LIBRARY_PATH", "/usr/local/nvidia/lib:/usr/local/nvidia/lib64", dagger.ContainerWithEnvVariableOpts{Expand: true}).
		WithEnvVariable("NVIDIA_VISIBLE_DEVICES", "all").
		WithEnvVariable("NVIDIA_DRIVER_CAPABILITIES", "compute,utility")

	opensearchUpdateService := opniPythonBase.
		Pipeline("Opensearch Update Service Image").
		WithDirectory(".", b.sources.Directory("aiops/")).
		WithExec([]string{"pip", "install", "-r", "requirements.txt"}).
		WithEntrypoint([]string{"python", "opni-opensearch-update-service/opensearch-update-service/app/main.py"})

	imageTargets := map[*config.ImageTarget]*dagger.Container{
		&b.Images.PythonBase:               opniPythonBase,
		&b.Images.PythonTorch:              opniPythonTorch,
		&b.Images.Opensearch.Opensearch:    opensearch,
		&b.Images.Opensearch.Dashboards:    opensearchDashboards,
		&b.Images.Opensearch.UpdateService: opensearchUpdateService,
	}

	eg, ctx := errgroup.WithContext(ctx)
	for target, container := range imageTargets {
		target, container := target, container
		if target.Push {
			eg.Go(func() error {
				ref, err := container.
					WithRegistryAuth(target.RegistryAuth()).
					Publish(ctx, target.Ref())
				if err != nil {
					return fmt.Errorf("failed to publish image %s: %w", target.Ref(), err)
				}
				fmt.Println("published image:", ref)
				return nil
			})
		}
	}

	return eg.Wait()
}
