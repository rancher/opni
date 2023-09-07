package cliutil

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/yaml"
)

var ErrAborted = errors.New("aborted by user")

type language interface {
	Marshal(t proto.Message) ([]byte, error)
	Unmarshal(data []byte, t proto.Message) error
	BeginComment() string
	FileExtension() string
	FileType() string
}

type jsonLanguage struct{}

var jsonMarshalOpts = protojson.MarshalOptions{
	Multiline:       true,
	Indent:          "  ",
	EmitUnpopulated: true,
	UseProtoNames:   true,
}

func (j jsonLanguage) Marshal(t proto.Message) ([]byte, error) {
	return jsonMarshalOpts.Marshal(t)
}

func (j jsonLanguage) Unmarshal(data []byte, t proto.Message) error {
	return protojson.Unmarshal(data, t)
}

func (j jsonLanguage) BeginComment() string {
	return "//"
}

func (j jsonLanguage) FileExtension() string {
	return "json"
}

func (j jsonLanguage) FileType() string {
	return "jsonc"
}

type yamlLanguage struct{}

func (y yamlLanguage) Marshal(t proto.Message) ([]byte, error) {
	jsonData, err := jsonMarshalOpts.Marshal(t)
	if err != nil {
		return nil, err
	}
	return yaml.JSONToYAML(jsonData)
}

func (y yamlLanguage) Unmarshal(data []byte, t proto.Message) error {
	jsonData, err := yaml.YAMLToJSON(data)
	if err != nil {
		return err
	}
	return protojson.Unmarshal(jsonData, t)
}

func (y yamlLanguage) BeginComment() string {
	return "#"
}

func (y yamlLanguage) FileExtension() string {
	return "yaml"
}

func (y yamlLanguage) FileType() string {
	return "yaml"
}

func determineBestEditingLanguage[T proto.Message](spec T) (language, error) {
	jsonData, err := jsonMarshalOpts.Marshal(spec)
	if err != nil {
		return nil, err
	}
	// json succeeded, now try converting to yaml
	_, err = yaml.JSONToYAML(jsonData)
	if err == nil {
		return yamlLanguage{}, nil
	}
	// yaml failed, but json succeeded, so use json
	return jsonLanguage{}, nil
}

var ErrNoEditor = fmt.Errorf("no available editor; please set the EDITOR environment variable and try again")

func EditInteractive[T proto.Message](spec T, comments ...string) (T, error) {
	lang, err := determineBestEditingLanguage(spec)
	if err != nil {
		return spec, err
	}
	for {
		extraComments := slices.Clone(comments)
		if err != nil {
			extraComments = []string{fmt.Sprintf("error: %v", err)}
		}
		var editedSpec T
		editedSpec, err = tryEdit(spec, lang, extraComments)
		if err != nil {
			if errors.Is(err, ErrAborted) || errors.Is(err, ErrNoEditor) {
				return editedSpec, err
			}
			continue
		}
		return editedSpec, nil
	}
}

func LoadFromFile[T proto.Message](spec T, path string) error {
	var f *os.File
	if path == "-" {
		f = os.Stdin
	} else {
		var err error
		f, err = os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
	}
	bytes, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	if len(bytes) == 0 {
		return fmt.Errorf("file is empty")
	}
	jsonData, err := yaml.YAMLToJSON(bytes)
	if err != nil {
		return fmt.Errorf("error parsing yaml/json: %w", err)
	}
	if err := protojson.Unmarshal(jsonData, spec); err != nil {
		return fmt.Errorf("error unmarshalling json: %w", err)
	}
	return nil
}

func tryEdit[T proto.Message](spec T, lang language, extraComments []string) (T, error) {
	for i, comment := range extraComments {
		if !strings.HasPrefix(comment, lang.BeginComment()) {
			extraComments[i] = lang.BeginComment() + " " + comment
		}
	}

	var nilT T
	inputData, err := lang.Marshal(spec)
	if err != nil {
		return nilT, err
	}

	// Add comments to the JSON
	comments := append([]string{
		lang.BeginComment() + " Edit the configuration below. Comments are ignored.",
		lang.BeginComment() + " If everything is deleted, the operation will be aborted.",
	}, extraComments...)
	specWithComments := strings.Join(append(comments, string(inputData)), "\n")

	// Create a temporary file for editing
	tmpFile, err := os.CreateTemp("", "opni-cli-temp-editing-*.json")
	if err != nil {
		return nilT, err
	}
	defer os.Remove(tmpFile.Name())

	// Write the JSON with comments to the temporary file
	if _, err := tmpFile.WriteString(specWithComments); err != nil {
		return nilT, err
	}
	if err := tmpFile.Close(); err != nil {
		return nilT, err
	}

	// Open the temporary file in the user's preferred editor
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editors := []string{"nvim", "vim", "vi"}
		for _, e := range editors {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}

	args := []string{tmpFile.Name()}
	if editor == "vim" || editor == "nvim" {
		// set syntax highlighting for editors other than vi
		if ft := lang.FileType(); ft != "" {
			args = append(args, fmt.Sprintf("+set ft=%s", ft))
		}
	}

	if _, err := exec.LookPath(editor); err != nil {
		return nilT, ErrNoEditor
	}

	cmd := exec.Command(editor, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nilT, fmt.Errorf("editor command failed: %w", err)
	}

	// Read the edited JSON
	editedBytes, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return nilT, err
	}

	editedBytes = bytes.TrimSpace(editedBytes)

	// Remove comments and empty lines
	editedLines := strings.Split(string(editedBytes), "\n")
	filteredLines := make([]string, 0, len(editedLines))
	for _, line := range editedLines {
		trimmedLine := strings.TrimSpace(line)
		if len(trimmedLine) > 0 && !strings.HasPrefix(trimmedLine, lang.BeginComment()) {
			filteredLines = append(filteredLines, line)
		}
	}

	// If everything is deleted, abort the operation
	if len(filteredLines) == 0 {
		return nilT, ErrAborted
	}

	editedSpec := spec.ProtoReflect().New()

	if err := lang.Unmarshal([]byte(strings.Join(filteredLines, "\n")), editedSpec.Interface()); err != nil {
		return nilT, err
	}

	return editedSpec.Interface().(T), nil
}
