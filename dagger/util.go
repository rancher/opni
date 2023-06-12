package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dagger.io/dagger"
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
	envVars := make(map[string]string)
	var longestKey, longestType, longestValue, longestEnv int
	specialCaseEnvs := config.SpecialCaseEnvVars(nil)
	for _, key := range keys {
		if len(key) > longestKey {
			longestKey = len(key)
		}
		env := fmt.Sprintf("%s%s", config.EnvPrefix, strings.ToUpper(strings.ReplaceAll(key, ".", "_")))
	OUTER:
		for _, sc := range specialCaseEnvs {
			for _, k := range sc.Keys {
				if k == key {
					env = fmt.Sprintf("%s, %s", env, sc.EnvVar)
					break OUTER
				}
			}
		}
		if len(env) > longestEnv {
			longestEnv = len(env)
		}
		envVars[key] = env
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
		if len(valueType) > longestType {
			longestType = len(valueType)
		}
		valueTypes[key] = valueType
		valueStr := fmt.Sprintf("%v", value)
		if len(valueStr) > longestValue {
			longestValue = len(valueStr)
		}
		values[key] = valueStr
	}
	// key | type | value | env var

	// print the header
	fmt.Printf("%s-+-%s-+-%s-+-%s\n", strings.Repeat("-", longestKey), strings.Repeat("-", longestType), strings.Repeat("-", longestValue), strings.Repeat("-", longestEnv))
	fmt.Printf("%-*s | %-*s | %-*s | %-*s\n", longestKey, "Key", longestType, "Type", longestValue, "Value", longestEnv, "Environment Variable")
	fmt.Printf("%s-+-%s-+-%s-+-%s\n", strings.Repeat("-", longestKey), strings.Repeat("-", longestType), strings.Repeat("-", longestValue), strings.Repeat("-", longestEnv))

	// print the values
	for _, key := range keys {
		fmt.Printf("%-*s | %-*s | %-*s | %-*s\n", longestKey, key, longestType, valueTypes[key], longestValue, values[key], longestEnv, envVars[key])
	}

	// print the footer
	fmt.Printf("%s-+-%s-+-%s-+-%s\n", strings.Repeat("-", longestKey), strings.Repeat("-", longestType), strings.Repeat("-", longestValue), strings.Repeat("-", longestEnv))
}
