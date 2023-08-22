package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"dagger.io/dagger"
	"github.com/go-playground/validator/v10"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/mitchellh/copystructure"
)

type BuilderConfig struct {
	Images   ImagesConfig   `koanf:"images"`
	Charts   ChartsConfig   `koanf:"charts"`
	Lint     bool           `koanf:"lint"`
	Test     bool           `koanf:"test"`
	Coverage CoverageConfig `koanf:"coverage"`
	Releaser ReleaseTarget  `koanf:"releaser"`
}

type ImagesConfig struct {
	Opni        ImageTarget      `koanf:"opni"`
	OpniMinimal ImageTarget      `koanf:"minimal"`
	Opensearch  OpensearchConfig `koanf:"opensearch"`
	PythonBase  ImageTarget      `koanf:"python-base"`
	PythonTorch ImageTarget      `koanf:"python-torch"`
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

type ReleaseTarget struct {
	Push bool       `koanf:"push"`
	OS   []string   `koanf:"os"`
	Arch []string   `koanf:"arch"`
	Repo string     `koanf:"repo" validate:"required_if=Push true"`
	Tag  string     `koanf:"tag" validate:"required_if=Push true"`
	Auth AuthConfig `koanf:"auth" validate:"required_if=Push true"`
}

type OpensearchConfig struct {
	Opensearch    ImageTarget `koanf:"opensearch"`
	Dashboards    ImageTarget `koanf:"dashboards"`
	UpdateService ImageTarget `koanf:"update-service"`

	Build OpensearchBuildConfig `koanf:"build" validate:"required_with=Opensearch Dashboards"`
}

type OpensearchBuildConfig struct {
	DashboardsVersion string `koanf:"dashboards-version"`
	OpensearchVersion string `koanf:"opensearch-version"`
	PluginVersion     string `koanf:"plugin-version"`
}

type CoverageConfig struct {
	Export bool `koanf:"export"`
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

func ValidationHelp() string {
	return fmt.Sprintf(`
  - If any image has 'push' set to true, that image's 'repo', 'tag', and 'auth' fields must also be set.
  - If any chart has 'push' set to true, that chart's 'repo', 'branch' (for non-OCI charts), and 'auth' fields must also be set.
  - If any image has 'tag-suffix' set, that image's 'tag' must also be set. Otherwise, 'tag-suffix' is optional.
  - Image 'repo' fields must not contain tags or digests.
  - Image 'tag' fields must be valid Docker image tags.
  - Image 'tag-suffix' fields must start with a '-' character and be followed by a valid Docker image tag.
`[1:])
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

func init() {
	copystructure.Copiers[reflect.TypeOf(dagger.Secret{})] = func(v any) (any, error) {
		return v, nil
	}
}

func AutoLoader(filename string) (koanf.Provider, koanf.Parser, koanf.Option) {
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

	return file.Provider(filename), parser, koanf.WithMergeFunc(mergeStrict)
}

// same as mergeStrict() from koanf/maps/maps.go with added support for merging
// empty []any slices with slices of a specific type.
func mergeStrict(src, dest map[string]any) error {
	for key, val := range src {
		// Does the key exist in the target map?
		// If no, add it and move on.
		bVal, ok := dest[key]
		if !ok {
			dest[key] = val
			continue
		}

		// If the incoming val is not a map, do a direct merge between the same types.
		if _, ok := val.(map[string]any); !ok {
			if reflect.TypeOf(dest[key]) == reflect.TypeOf(val) {
				dest[key] = val
			} else {
				// Handle the special case where empty slices are decoded as []any causing
				// a type mismatch when merging with a slice of a specific type.
				if reflect.TypeOf(dest[key]).Kind() == reflect.Slice &&
					reflect.TypeOf(val) == reflect.TypeOf(([]any)(nil)) &&
					reflect.ValueOf(dest[key]).Len() == 0 {
					dest[key] = reflect.MakeSlice(reflect.TypeOf(dest[key]), 0, 0).Interface()
					for _, v := range val.([]any) {
						dest[key] = reflect.Append(reflect.ValueOf(dest[key]), reflect.ValueOf(v)).Interface()
					}
					continue
				}
				return fmt.Errorf("incorrect types at key %v, type %T != %T", key, dest[key], val)
			}
			continue
		}

		// The source key and target keys are both maps. Merge them.
		switch v := bVal.(type) {
		case map[string]any:
			if err := mergeStrict(val.(map[string]any), v); err != nil {
				return err
			}
		default:
			dest[key] = val
		}
	}
	return nil
}

func referenceName(fl validator.FieldLevel) bool {
	if fl.Field().IsZero() {
		return true
	}
	matches := referenceRegexp.FindStringSubmatch(fl.Field().String())
	return len(matches) == 4 && matches[0] == matches[1]
}

func referenceTag(fl validator.FieldLevel) bool {
	if fl.Field().IsZero() {
		return true
	}
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

func Marshal(k *koanf.Koanf, outputFormat string) []byte {
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
		data = []byte(renderTable(k))
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
	return data
}

func renderTable(k *koanf.Koanf) string {
	keys := k.Keys()
	values := k.All()
	valueTypes := make(map[string]string)
	envVars := make(map[string][]any)
	specialCaseEnvs := SpecialCaseEnvVars(nil)
	for _, key := range keys {
		env := fmt.Sprintf("%s%s", EnvPrefix, strings.ToUpper(strings.ReplaceAll(key, ".", "_")))
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
	return w.Render()
}
