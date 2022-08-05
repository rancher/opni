package capabilities

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

type InstallerTemplateSpec struct {
	UserInstallerTemplateSpec
	ServerInstallerTemplateSpec
}
type UserInstallerTemplateSpec struct {
	Token string
	Pin   string
}

type ServerInstallerTemplateSpec struct {
	Address string
	Port    string
}

var installerTemplate *template.Template

func init() {
	fm := sprig.TxtFuncMap()
	fm["arg"] = Arg
	fm["value"] = func() string {
		return "{{value}}"
	}
	installerTemplate = template.New("installer").Funcs(fm)
}

type ArgKind string

const (
	ArgKindInput  ArgKind = "input"
	ArgKindSelect ArgKind = "select"
	ArgKindToggle ArgKind = "toggle"
)

type ArgSpec struct {
	Kind    ArgKind      `json:"kind"`
	Input   *InputSpec   `json:"input,omitempty"`
	Select  *SelectSpec  `json:"select,omitempty"`
	Toggle  *ToggleSpec  `json:"toggle,omitempty"`
	Options []OptionSpec `json:"options,omitempty"`
}

type InputSpec struct {
	Label string `json:"label"`
}

type SelectSpec struct {
	Label string   `json:"label"`
	Items []string `json:"items"`
}

type ToggleSpec struct {
	Label string `json:"label"`
}

type OptionSpec struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}

// Arg generates a JSON string that the UI can parse to generate input controls
// (text boxes, combo boxes, etc.) for the user to fill in. The values the user
// provides will be substituted into the install command for the user to copy.
//
// Usage: {{ arg <kind> [args...] [+option ...] }}
// The args are specific to each kind of input control.
//
// Kinds:
//
//	"input": A text box.
//	  Arguments:
//	    1: Label
//	"select": A combo box.
//	  Arguments:
//	    1: Label
//	    2+: Combo box items
//	"toggle": A checkbox.
//	  Arguments:
//	    1: Label
//
// Options:
//
//	"+format:<value>": Controls how the value is substituted into the install
//	                   command. Within the format text, {{ value }} will be
//	                   replaced with the user input. Defaults to '{{ value }}'
//	"+required": Marks the input is required.
//	"+omitEmpty": If the value is "falsy" ('', 'false', etc), the argument
//	              will be omitted from the install command.
//	"+default:<value>": Adds a default value to the input.
func Arg(kind ArgKind, args ...string) (string, error) {
	opts, remaining, err := extractOptions(args...)
	if err != nil {
		return "", err
	}
	argSpec := ArgSpec{
		Kind:    kind,
		Options: opts,
	}
	switch args := remaining; kind {
	case ArgKindInput:
		if len(args) == 0 {
			return "", fmt.Errorf(`usage: {{ arg "input" "Label" ["+option" ...] }}`)
		}
		argSpec.Input = &InputSpec{
			Label: args[0],
		}
	case ArgKindSelect:
		if len(args) < 2 {
			return "", fmt.Errorf(`usage: {{ arg "select" "Label" "Item 1" ["Item 2" ...] ["+option" ...] }}`)
		}
		argSpec.Select = &SelectSpec{
			Label: args[0],
			Items: args[1:],
		}
	case ArgKindToggle:
		if len(args) == 0 {
			return "", fmt.Errorf(`usage: {{ arg "toggle" "Label" ["+option" ...] }}`)
		}
		argSpec.Toggle = &ToggleSpec{
			Label: args[0],
		}
	}
	argJson, err := json.Marshal(argSpec)
	if err != nil {
		return "", fmt.Errorf("failed to marshal arg spec: %w", err)
	}
	return fmt.Sprintf("%%%s%%", argJson), nil
}

func extractOptions(options ...string) ([]OptionSpec, []string, error) {
	var optionSpecs []OptionSpec
	var remaining []string
	for _, option := range options {
		if option != "" && option[0] == '+' {
			switch option := option[1:]; {
			case strings.HasPrefix(option, "required"):
				if strings.Contains(option, ":") {
					return nil, nil, fmt.Errorf("option %q does not accept a value", option)
				}
				optionSpecs = append(optionSpecs, OptionSpec{
					Name: "required",
				})
			case strings.HasPrefix(option, "omitEmpty"):
				if strings.Contains(option, ":") {
					return nil, nil, fmt.Errorf("option %q does not accept a value", option)
				}
				optionSpecs = append(optionSpecs, OptionSpec{
					Name: "omitEmpty",
				})
			case strings.HasPrefix(option, "default"):
				_, value, ok := strings.Cut(option, ":")
				if !ok {
					return nil, nil, errors.New("usage: +default:value")
				}
				optionSpecs = append(optionSpecs, OptionSpec{
					Name:  "default",
					Value: value,
				})
			case strings.HasPrefix(option, "format"):
				_, value, ok := strings.Cut(option, ":")
				if !ok {
					return nil, nil, errors.New("usage: +format:value")
				}
				optionSpecs = append(optionSpecs, OptionSpec{
					Name:  "format",
					Value: value,
				})
			default:
				return nil, nil, fmt.Errorf("unknown option: %q", option)
			}
		} else {
			remaining = append(remaining, option)
		}
	}
	return optionSpecs, remaining, nil
}

func RenderInstallerCommand(tmpl string, spec InstallerTemplateSpec) (string, error) {
	t, err := installerTemplate.Parse(tmpl)
	if err != nil {
		return "", err
	}
	buf := new(bytes.Buffer)
	err = t.Execute(buf, spec)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
