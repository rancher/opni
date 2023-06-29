package cliutil

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var ErrAborted = errors.New("aborted by user")

func EditInteractive[T proto.Message](spec T, id ...string) (T, error) {
	var err error
	for {
		var extraComments []string
		if len(id) > 0 {
			extraComments = []string{fmt.Sprintf("[id: %s]", id[0])}
		}
		if err != nil {
			extraComments = []string{fmt.Sprintf("error: %v", err)}
		}
		editedSpec, err := tryEdit(spec, extraComments)
		if err != nil {
			if errors.Is(err, ErrAborted) {
				return editedSpec, err
			}
			continue
		}
		return editedSpec, nil
	}
}

func tryEdit[T proto.Message](spec T, extraComments []string) (T, error) {
	jsonData, err := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: true,
	}.Marshal(spec)

	var nilT T
	if err != nil {
		return nilT, err
	}

	for i, comment := range extraComments {
		if !strings.HasPrefix(comment, "//") {
			extraComments[i] = "// " + comment
		}
	}

	// Add comments to the JSON
	comments := append([]string{
		"// Edit the configuration below. Comments are ignored.",
		"// If everything is deleted, the operation will be aborted.",
	}, extraComments...)
	specWithComments := strings.Join(append(comments, string(jsonData)), "\n")

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
		editor = "vim" // Default to 'vim' if no editor is set
	}

	args := []string{tmpFile.Name()}
	if editor != "vi" {
		// set jsonc syntax highlighting for editors other than vi
		args = append(args, "+set ft=jsonc")
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
		if !strings.HasPrefix(strings.TrimSpace(line), "//") {
			filteredLines = append(filteredLines, line)
		}
	}

	// If everything is deleted, abort the operation
	if len(filteredLines) == 0 {
		return nilT, ErrAborted
	}

	editedSpec := spec.ProtoReflect().New()

	// Unmarshal the edited JSON into a new spec
	if err := protojson.Unmarshal([]byte(strings.Join(filteredLines, "\n")), editedSpec.Interface()); err != nil {
		return nilT, err
	}

	return editedSpec.Interface().(T), nil
}
