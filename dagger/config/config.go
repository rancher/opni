package config

import (
	"fmt"
	"reflect"
	"strings"

	"dagger.io/dagger"
	"github.com/mitchellh/copystructure"
	"github.com/spf13/pflag"
)

type SpecialCaseEnv struct {
	EnvVar    string
	Keys      []string
	Converter func(k, v string) any
}

func SpecialCaseEnvVars(client *dagger.Client) []SpecialCaseEnv {
	fn := func(k, v string) any {
		return client.SetSecret(k, v)
	}

	return []SpecialCaseEnv{
		{
			EnvVar: "DOCKER_USERNAME",
			Keys: []string{
				"images.opni.auth.username",
				"images.minimal.auth.username",
				"images.opensearch.opensearch.auth.username",
				"images.opensearch.dashboards.auth.username",
				"images.opensearch.update-service.auth.username",
				"charts.oci.auth.username",
			},
			Converter: func(_, v string) any {
				return v
			},
		},
		{
			EnvVar: "DOCKER_PASSWORD",
			Keys: []string{
				"images.opni.auth.secret",
				"images.minimal.auth.secret",
				"images.opensearch.opensearch.auth.secret",
				"images.opensearch.dashboards.auth.secret",
				"images.opensearch.update-service.auth.secret",
				"charts.oci.auth.secret",
			},
			Converter: fn,
		},
		{
			EnvVar: "GH_TOKEN",
			Keys: []string{
				"charts.git.auth.secret",
			},
			Converter: fn,
		},
	}
}

type BuilderConfig struct {
	Images ImagesConfig `koanf:"images"`
	Charts ChartsConfig `koanf:"charts"`
	Lint   bool         `koanf:"lint"`
}

type ImagesConfig struct {
	Opni        ImageTarget      `koanf:"opni"`
	OpniMinimal ImageTarget      `koanf:"minimal"`
	Opensearch  OpensearchConfig `koanf:"opensearch"`
}

type ImageTarget struct {
	Push      bool       `koanf:"push"`
	Repo      string     `koanf:"repo" validate:"required_if=Push true"`
	Tag       string     `koanf:"tag" validate:"required_if=Push true"`
	TagSuffix string     `koanf:"tag-suffix" validate:"excluded_if=Tag ''"`
	Auth      AuthConfig `koanf:"auth" validate:"required_if=Push true"`
}

type AuthConfig struct {
	Username string         `koanf:"username"`
	Email    string         `koanf:"email"`
	Secret   *dagger.Secret `koanf:"secret" validate:"required"`
}

type ChartsConfig struct {
	OCI OCIChartTarget `koanf:"oci"`
	Git ChartTarget    `koanf:"git"`
}

type OCIChartTarget struct {
	Push []string   `koanf:"push"`
	Repo string     `koanf:"repo" validate:"required_with=Push"`
	Auth AuthConfig `koanf:"auth" validate:"required_with=Push"`
}

type ChartTarget struct {
	Push   bool       `koanf:"push"`
	Export bool       `koanf:"export"`
	Repo   string     `koanf:"repo" validate:"required_if=Push true"`
	Branch string     `koanf:"branch" validate:"required_if=Push true"`
	Auth   AuthConfig `koanf:"auth" validate:"required_if=Push true"`
}

type OpensearchConfig struct {
	Opensearch    ImageTarget `koanf:"opensearch"`
	Dashboards    ImageTarget `koanf:"dashboards"`
	UpdateService ImageTarget `koanf:"update-service"`

	Build struct {
		DashboardsVersion string `koanf:"dashboards-version" validate:"required_with=Opensearch Dashboards"`
		OpensearchVersion string `koanf:"opensearch-version" validate:"required_with=Opensearch"`
		PluginVersion     string `koanf:"plugin-version" validate:"required_with=Opensearch Dashboards"`
	} `koanf:"build"`
}

func BuildFlagSet(t reflect.Type, prefix ...string) *pflag.FlagSet {
	fs := pflag.NewFlagSet("", pflag.ExitOnError)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tagValue := field.Tag.Get("koanf")
		if tagValue == "" {
			continue
		}
		flagName := strings.Join(append(prefix, tagValue), ".")
		if field.Type == reflect.TypeOf(&dagger.Secret{}) {
			continue
		}
		switch kind := field.Type.Kind(); kind {
		case reflect.String:
			fs.String(flagName, "", tagValue)
		case reflect.Slice:
			switch elemKind := field.Type.Elem().Kind(); elemKind {
			case reflect.String:
				fs.StringSlice(flagName, nil, tagValue)
			default:
				panic("unimplemented: []" + elemKind.String())
			}
		case reflect.Bool:
			fs.Bool(flagName, false, tagValue)
		case reflect.Struct:
			fs.AddFlagSet(BuildFlagSet(field.Type, append(prefix, tagValue)...))
		default:
			panic("unimplemented: " + kind.String())
		}
	}
	return fs
}

func (t *ImageTarget) RegistryAuth() (address string, username string, secret *dagger.Secret) {
	address = t.Repo
	username = t.Auth.Username
	secret = t.Auth.Secret
	return
}

func (t *ImageTarget) Ref() string {
	return fmt.Sprintf("%s:%s%s", t.Repo, t.Tag, t.TagSuffix)
}

type CacheVolume struct {
	*dagger.CacheVolume
	Path string
}

type Caches struct {
	GoMod       func() (string, *dagger.CacheVolume)
	GoBuild     func() (string, *dagger.CacheVolume)
	GoBin       func() (string, *dagger.CacheVolume)
	Mage        func() (string, *dagger.CacheVolume)
	Yarn        func() (string, *dagger.CacheVolume)
	NodeModules func() (string, *dagger.CacheVolume)
}

func init() {
	copystructure.Copiers[reflect.TypeOf(dagger.Secret{})] = func(v any) (any, error) {
		return v, nil
	}
}
