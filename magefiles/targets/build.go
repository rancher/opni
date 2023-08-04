package targets

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

var verboseFlag = fmt.Sprintf("-v=%t", mg.Verbose())

type Build mg.Namespace

// Builds the opni binary and plugins
func (Build) All(ctx context.Context) {
	ctx, tr := Tracer.Start(ctx, "target.build")
	defer tr.End()

	mg.CtxDeps(ctx, Build.Opni, Build.Plugins)
}

// Compiles all go packages except those named 'main'
func (Build) Archives(ctx context.Context) error {
	_, tr := Tracer.Start(ctx, "target.build.archives")
	defer tr.End()

	return buildArchive("./...")
}

// Builds the custom linter plugin
func (Build) Linter(ctx context.Context) error {
	_, tr := Tracer.Start(ctx, "target.build.linter")
	defer tr.End()

	return buildMainPackage(buildOpts{
		Path:   "./internal/cmd/lint",
		Output: "bin/lint",
	})
}

// Build the typescript service generator plugin
func (Build) TypescriptServiceGenerator() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	webPath := path.Join(cwd, "web")
	command := exec.Command("yarn", "build:service-generator")
	command.Dir = webPath
	command.Stderr = os.Stderr
	command.Stdout = os.Stdout
	command.Stdin = os.Stdin
	
	return command.Run()
}

type buildOpts struct {
	Path   string
	Output string
	Tags   []string
	Debug  bool
}

func buildMainPackage(opts buildOpts) error {
	tag, _ := os.LookupEnv("BUILD_VERSION")
	tag = strings.TrimSpace(tag)

	version := "unversioned"
	if tag != "" {
		version = tag
	}

	args := []string{
		"go", "build", "-v",
	}
	if !opts.Debug {
		args = append(args,
			"-ldflags", fmt.Sprintf("-w -s -X github.com/rancher/opni/pkg/versions.Version=%s", version),
			"-trimpath",
		)
	}
	args = append(args, "-o", opts.Output)
	if len(opts.Tags) > 0 {
		args = append(args, fmt.Sprintf("-tags=%s", strings.Join(opts.Tags, ",")))
	}

	// disable vcs stamping inside git worktrees if the linked git directory doesn't exist
	dotGit, err := os.Stat(".git")
	if err == nil && !dotGit.IsDir() {
		fmt.Println("disabling vcs stamping inside worktree")
		args = append(args, "-buildvcs=false")
	}

	args = append(args, opts.Path)

	return sh.RunWith(map[string]string{"CGO_ENABLED": "0"}, args[0], args[1:]...)
}

func buildArchive(path string) error {
	return sh.RunWith(map[string]string{
		"CGO_ENABLED": "0",
	}, mg.GoCmd(), "build", verboseFlag, "-buildmode=archive", "-trimpath", "-tags=nomsgpack", path)
}
