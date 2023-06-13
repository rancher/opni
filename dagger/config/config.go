package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"dagger.io/dagger"
	"github.com/go-playground/validator/v10"
	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/mitchellh/copystructure"
	"github.com/spf13/pflag"
)

var (
	// generated from docker/distribution
	referenceRegexp = regexp.MustCompile(`^((?:(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])(?:(?:\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))+)?(?::[0-9]+)?/)?[a-z0-9]+(?:(?:(?:[._]|__|[-]*)[a-z0-9]+)+)?(?:(?:/[a-z0-9]+(?:(?:(?:[._]|__|[-]*)[a-z0-9]+)+)?)+)?)(?::([\w][\w.-]{0,127}))?(?:@([A-Za-z][A-Za-z0-9]*(?:[-_+.][A-Za-z][A-Za-z0-9]*)*[:][[:xdigit:]]{32,}))?$`)
	tagRegexp       = regexp.MustCompile(`[\w][\w.-]{0,127}`)
)

const EnvPrefix = "_OPNI_"

type SpecialCaseEnv struct {
	EnvVar    string
	Keys      []string
	Converter func(k, v string) any
}

func SpecialCaseEnvVars(client *dagger.Client) []SpecialCaseEnv {
	secret := func(k, v string) any {
		return client.SetSecret(k, v)
	}
	plaintext := func(_, v string) any {
		return v
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
				"images.python-base.auth.username",
				"charts.oci.auth.username",
			},
			Converter: plaintext,
		},
		{
			EnvVar: "DOCKER_PASSWORD",
			Keys: []string{
				"images.opni.auth.secret",
				"images.minimal.auth.secret",
				"images.opensearch.opensearch.auth.secret",
				"images.opensearch.dashboards.auth.secret",
				"images.opensearch.update-service.auth.secret",
				"images.python-base.auth.secret",
				"charts.oci.auth.secret",
			},
			Converter: secret,
		},
		{
			EnvVar: "GH_TOKEN",
			Keys: []string{
				"charts.git.auth.secret",
			},
			Converter: secret,
		},
		{
			EnvVar: "DRONE_BRANCH",
			Keys: []string{
				"images.opni.tag",
				"images.minimal.tag",
				"images.opensearch.opensearch.tag",
				"images.opensearch.dashboards.tag",
				"images.opensearch.update-service.tag",
			},
			Converter: plaintext,
		},
		{
			EnvVar: "DRONE_TAG", // if set, will override DRONE_BRANCH
			Keys: []string{
				"images.opni.tag",
				"images.minimal.tag",
				"images.opensearch.opensearch.tag",
				"images.opensearch.dashboards.tag",
				"images.opeesearch.update-service.tag",
			},
			Converter: plaintext,
		},
	}
}

type BuilderConfig struct {
	Images ImagesConfig `koanf:"images"`
	Charts ChartsConfig `koanf:"charts"`
	Lint   bool         `koanf:"lint"`
	Test   bool         `koanf:"test"`
}

type ImagesConfig struct {
	Opni        ImageTarget      `koanf:"opni"`
	OpniMinimal ImageTarget      `koanf:"minimal"`
	Opensearch  OpensearchConfig `koanf:"opensearch"`
	PythonBase  ImageTarget      `koanf:"python-base"`
}

type ImageTarget struct {
	Push           bool       `koanf:"push"`
	Repo           string     `koanf:"repo" validate:"required_if=Push true,reference-name-only"`
	Tag            string     `koanf:"tag" validate:"required_if=Push true,reference-tag"`
	TagSuffix      string     `koanf:"tag-suffix" validate:"excluded_if=Tag '',reference-tag-suffix"`
	AdditionalTags []string   `koanf:"additional-tags" validate:"dive,reference-tag"`
	Auth           AuthConfig `koanf:"auth" validate:"required_if=Push true"`
}

type AuthConfig struct {
	Username string         `koanf:"username"`
	Email    string         `koanf:"email"`
	Secret   *dagger.Secret `koanf:"secret"`
}

type ChartsConfig struct {
	OCI OCIChartTarget `koanf:"oci"`
	Git ChartTarget    `koanf:"git"`
}

type OCIChartTarget struct {
	Push bool       `koanf:"push"`
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

func Validate(conf *BuilderConfig) error {
	v := validator.New()
	v.RegisterStructValidation(func(sl validator.StructLevel) {
		ac := sl.Current().Interface().(AuthConfig)
		if ac.Secret == nil {
			if !sl.Parent().FieldByName("Push").IsZero() {
				sl.ReportError(ac.Secret, "secret", "secret", "required_if", "Push")
			}
		}
	}, AuthConfig{})
	v.RegisterValidation("reference-name-only", referenceName)
	v.RegisterValidation("reference-tag", referenceTag)
	v.RegisterValidation("reference-tag-suffix", referenceTagSuffix)
	v.RegisterTagNameFunc(func(sf reflect.StructField) string {
		return strings.SplitN(sf.Tag.Get("koanf"), ",", 2)[0]
	})
	return v.Struct(conf)
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

func (t *ImageTarget) AdditionalRefs() []string {
	var refs []string
	for _, tag := range t.AdditionalTags {
		if tag == "" {
			continue
		}
		refs = append(refs, fmt.Sprintf("%s:%s%s", t.Repo, tag, t.TagSuffix))
	}
	return refs
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
	TestBin     func() (string, *dagger.CacheVolume)
}

func init() {
	copystructure.Copiers[reflect.TypeOf(dagger.Secret{})] = func(v any) (any, error) {
		return v, nil
	}
}

func AutoLoader(filename string) (koanf.Provider, koanf.Parser) {
	var parser koanf.Parser

	if _, err := os.Stat(filename); err != nil {
		if filepath.Ext(filename) == "" {
			matches, err := filepath.Glob(fmt.Sprintf("%s.*", filename))
			if err != nil {
				panic(err)
			}
			if len(matches) == 0 {
				fmt.Fprintf(os.Stderr, "no config file found for %q\n", filename)
			}
			if len(matches) > 1 {
				fmt.Fprintf(os.Stderr, "ambiguous config file path %q matches %v\n", filename, matches)
			}

			filename = matches[0]
		}
	}

	if _, err := os.Stat(filename); err != nil {
		fmt.Fprintf(os.Stderr, "config file %q not found\n", filename)
		os.Exit(1)
	}

	switch filepath.Ext(filename) {
	case ".yaml", ".yml":
		parser = yaml.Parser()
	case ".json":
		parser = json.Parser()
	case ".toml":
		parser = toml.Parser()
	default:
		fmt.Fprintf(os.Stderr, "config file %q has unsupported extension (supported: .yaml, .json, .toml)\n", filename)
		os.Exit(1)
	}

	return file.Provider(filename), parser
}

func referenceName(fl validator.FieldLevel) bool {
	matches := referenceRegexp.FindStringSubmatch(fl.Field().String())
	return len(matches) == 4 && matches[0] == matches[1]
}

func referenceTag(fl validator.FieldLevel) bool {
	return tagRegexp.MatchString(fl.Field().String())
}

func referenceTagSuffix(fl validator.FieldLevel) bool {
	str := fl.Field().String()
	if len(str) == 0 {
		return true
	}
	if str[0] != '-' {
		return false
	}
	return tagRegexp.MatchString(str[1:])
}
