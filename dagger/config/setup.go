package config

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"slices"

	"github.com/AlecAivazis/survey/v2"
	"github.com/bmatcuk/doublestar/v4"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

type answerWriter struct {
	bc            BuilderConfig
	k             *koanf.Koanf
	uninterpreted map[string]any
}

func (w *answerWriter) WriteAnswer(field string, value any) error {
	if field[0] == '&' {
		field = field[1:]
		if strings.Contains(field, "*") {
			for _, key := range w.k.Keys() {
				if ok, _ := doublestar.Match(field, key); ok {
					if err := w.k.Set(key, value); err != nil {
						return err
					}
				}
			}
		} else {
			if err := w.k.Set(field, value); err != nil {
				return err
			}
		}
	}
	w.uninterpreted[field] = value
	return nil
}

func RunSetup() {
	conf := answerWriter{
		bc: BuilderConfig{
			Images: ImagesConfig{
				OpniMinimal: ImageTarget{
					TagSuffix: "-minimal",
				},
				PythonBase: ImageTarget{
					Tag: "3.9",
				},
				PythonTorch: ImageTarget{
					Tag:       "3.9",
					TagSuffix: "-torch",
				},
				Opensearch: OpensearchConfig{
					Build: OpensearchBuildConfig{
						DashboardsVersion: "2.8.0",
						OpensearchVersion: "2.8.0",
						PluginVersion:     "0.11.2",
					},
				},
			},
		},
		k:             koanf.New("."),
		uninterpreted: make(map[string]any),
	}
	conf.k.Load(structs.Provider(conf.bc, "koanf"), nil)

	const (
		opniImageName                    = "Opni Image"
		opniMinimalImageName             = "Opni Minimal Image"
		opensearchImageName              = "Opensearch Image"
		opensearchDashboardsImageName    = "Opensearch Dashboards Image"
		opensearchUpdateServiceImageName = "Opensearch Update Service Image"
		pythonBaseImageName              = "Python Base Image"
		pythonTorchImageName             = "Python Torch Image"
	)
	imageFields := map[string]*ImageTarget{
		opniImageName:                    &conf.bc.Images.Opni,
		opniMinimalImageName:             &conf.bc.Images.OpniMinimal,
		opensearchImageName:              &conf.bc.Images.Opensearch.Opensearch,
		opensearchDashboardsImageName:    &conf.bc.Images.Opensearch.Dashboards,
		opensearchUpdateServiceImageName: &conf.bc.Images.Opensearch.UpdateService,
		pythonBaseImageName:              &conf.bc.Images.PythonBase,
		pythonTorchImageName:             &conf.bc.Images.PythonTorch,
	}
	imageNames := map[string]string{
		opniImageName:                    "opni",
		opniMinimalImageName:             "opni",
		opensearchImageName:              "opensearch",
		opensearchDashboardsImageName:    "opensearch-dashboards",
		opensearchUpdateServiceImageName: "opensearch-update-service",
		pythonBaseImageName:              "opni-python-base",
		pythonTorchImageName:             "opni-python-base",
	}

	questions := []*survey.Question{
		{
			Name: "format",
			Prompt: &survey.Select{
				Message: "Select a configuration format:",
				Options: []string{"toml", "yaml", "json"},
				Default: "toml",
			},
		},
		{
			Name: "output",
			Prompt: &survey.Input{
				Message: "Enter a filename where the configuration will be saved:",
				Suggest: func(toComplete string) []string {
					if len(toComplete) == 0 {
						return []string{"dev/dagger.toml"}
					}
					return nil
				},
			},
			Validate: func(ans interface{}) error {
				path, err := expandPath(ans.(string))
				if err != nil {
					return err
				}
				// ensure the file doesn't exist
				if info, err := os.Stat(path); err == nil {
					return fmt.Errorf("%s already exists, please choose a different path", info.Name())
				}
				// ensure the directory exists
				if _, err := os.Stat(filepath.Dir(path)); err != nil {
					return fmt.Errorf("directory %s does not exist, please create it first.", filepath.Dir(path))
				}
				return nil
			},
			Transform: func(ans interface{}) interface{} {
				path, err := expandPath(ans.(string))
				if err != nil {
					return ans
				}
				return path
			},
		},
		{
			Name: "&*.repo",
			Prompt: &survey.Input{
				Message: "Enter a docker registry where images will be pushed:",
				Suggest: func(toComplete string) []string {
					if len(toComplete) == 0 {
						return []string{"docker.io/username"}
					}
					return nil
				},
			},
			Validate: func(ans interface{}) error {
				matches := referenceRegexp.FindStringSubmatch(ans.(string))
				if len(matches) == 4 && matches[0] == matches[1] {
					return nil
				}
				return errors.New("Not a valid registry name, expecting something like 'docker.io/username'")
			},
		},
		{
			Name: "&*.tag",
			Prompt: &survey.Input{
				Message: "Enter a default image tag:",
				Suggest: func(toComplete string) []string {
					if len(toComplete) == 0 {
						return []string{"latest"}
					}
					return nil
				},
			},
			Validate: func(ans interface{}) error {
				if tagRegexp.MatchString(ans.(string)) {
					return nil
				}
				return errors.New("Not a valid tag, expecting something like 'latest' or 'v1.0.0'")
			},
		},
		{
			Name: "images",
			Prompt: &survey.MultiSelect{
				Message: "Which images would you like to build?",
				Options: []string{
					opniImageName,
					opniMinimalImageName,
					opensearchImageName,
					opensearchDashboardsImageName,
					opensearchUpdateServiceImageName,
					pythonBaseImageName,
					pythonTorchImageName,
				},
				PageSize: len(imageNames),
			},
		},
		{
			Name: "&*.push",
			Prompt: &survey.Confirm{
				Message: "Push images?",
				Default: true,
			},
		},
		{
			Name: "&lint",
			Prompt: &survey.Confirm{
				Message: "Run linter?",
				Default: true,
			},
		},
		{
			Name: "&test",
			Prompt: &survey.Confirm{
				Message: "Run tests?",
				Default: true,
			},
		},
		{
			Name: "&coverage.export",
			Prompt: &survey.Confirm{
				Message: "Generate coverage reports?",
				Default: false,
			},
		},
	}

	err := survey.Ask(questions, &conf)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	selectedImages := conf.uninterpreted["images"].([]survey.OptionAnswer)
	var selectedImageStrings []string
	for _, img := range selectedImages {
		selectedImageStrings = append(selectedImageStrings, img.Value)
	}
	for imgName, field := range imageFields {
		if !slices.Contains(selectedImageStrings, imgName) {
			*field = ImageTarget{}
		} else {
			field.Repo = path.Join(field.Repo, imageNames[imgName])
		}
	}

	filename := conf.uninterpreted["output"].(string)
	format := conf.uninterpreted["format"].(survey.OptionAnswer)

	data := Marshal(conf.k, format.Value)
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		fmt.Println(err.Error())
		return
	}

	fmt.Printf("New configuration file written to %s\n", filename)
}

func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get home directory: %w", err)
		}
		path = strings.Replace(path, "~", home, 1)
	}
	path = filepath.Clean(os.ExpandEnv(path))
	return path, nil
}
