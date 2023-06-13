package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dagger.io/dagger"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/v2"
	"github.com/rancher/opni/dagger/config"
	"github.com/rancher/opni/dagger/images"
)

func (b *Builder) bin(paths ...string) string {
	return filepath.Join(append([]string{b.workdir, "bin"}, paths...)...)
}

func (b *Builder) nodeBase() *dagger.Container {
	return images.NodeBase(b.client).
		With(b.caches.Yarn).
		WithEnvVariable("YARN_CACHE_FOLDER", "/cache/yarn").
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
	return []string{"/go/bin/mage", "-v", target}, opts[0]
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

func printConfig(k *koanf.Koanf, outputFormat string) {
	var data []byte
	var err error
	if outputFormat != "table" {
		k = k.Copy()
		for _, key := range k.Keys() {
			if strings.Contains(key, "secret") {
				k.Delete(key)
			}
		}
	}
	switch outputFormat {
	case "table":
		printConfigTable(k)
		return
	case "json":
		data, err = k.Marshal(json.Parser())
	case "yaml":
		data, err = k.Marshal(yaml.Parser())
	case "toml":
		data, err = k.Marshal(toml.Parser())
	default:
		fmt.Fprintf(os.Stderr, "unknown output format: %s\n", outputFormat)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling config: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(data))
}

func printConfigTable(k *koanf.Koanf) {
	keys := k.Keys()
	values := k.All()
	valueTypes := make(map[string]string)
	envVars := make(map[string][]any)
	specialCaseEnvs := config.SpecialCaseEnvVars(nil)
	for _, key := range keys {
		env := fmt.Sprintf("%s%s", config.EnvPrefix, strings.ToUpper(strings.ReplaceAll(key, ".", "_")))
		envVars[key] = []any{env}
	OUTER:
		for _, sc := range specialCaseEnvs {
			for _, k := range sc.Keys {
				if k == key {
					envVars[key] = append(envVars[key], sc.EnvVar)
					break OUTER
				}
			}
		}
	}
	for key, value := range values {
		if strings.Contains(key, "secret") {
			valueTypes[key] = "string"
			if value != nil {
				values[key] = "<secret>"
			} else {
				values[key] = ""
			}
			continue
		}
		valueType := fmt.Sprintf("%T", value)
		valueTypes[key] = valueType
		valueStr := fmt.Sprintf("%v", value)
		values[key] = valueStr
	}
	w := table.NewWriter()

	maxNumEnvVars := 0
	for _, envs := range envVars {
		if len(envs) > maxNumEnvVars {
			maxNumEnvVars = len(envs)
		}
	}
	header := []any{"Key", "Type", "Value"}
	for i := 0; i < maxNumEnvVars; i++ {
		header = append(header, "Environment Variables")
	}
	w.AppendHeader(header, table.RowConfig{AutoMerge: true})
	for _, key := range keys {
		w.AppendRow(append(table.Row{key, valueTypes[key], values[key]}, envVars[key]...))
	}
	fmt.Println(w.Render())
}

func SetupCaches(client *dagger.Client, cacheMode string) config.Caches {
	if _, ok := os.LookupEnv("CI"); ok {
		cacheMode = CacheModeNone
	}
	identity := func(ctr *dagger.Container) *dagger.Container { return ctr }
	switch cacheMode {
	case CacheModeVolumes:
		return config.Caches{
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
		return config.Caches{
			GoMod:       identity,
			GoBuild:     identity,
			GoBin:       identity,
			Mage:        identity,
			Yarn:        identity,
			NodeModules: identity,
		}
	}
}
