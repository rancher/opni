package config

import (
	"fmt"
	"reflect"
	"strings"

	"dagger.io/dagger"
	"github.com/spf13/pflag"
)

type BuilderConfig struct {
	Images ImagesConfig `koanf:"images"`
	Charts ChartsConfig `koanf:"charts"`
	Lint   bool         `koanf:"lint"`
}

type ImagesConfig struct {
	Opni        ImageTarget `koanf:"opni"`
	OpniMinimal ImageTarget `koanf:"minimal"`
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
	Push bool       `koanf:"push"`
	Repo string     `koanf:"repo" validate:"required_if=Push true"`
	Auth AuthConfig `koanf:"auth" validate:"required_if=Push true"`
}

type ChartTarget struct {
	Push   bool       `koanf:"push"`
	Export bool       `koanf:"export"`
	Repo   string     `koanf:"repo" validate:"required_if=Push true"`
	Branch string     `koanf:"branch" validate:"required_if=Push true"`
	Auth   AuthConfig `koanf:"auth" validate:"required_if=Push true"`
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
