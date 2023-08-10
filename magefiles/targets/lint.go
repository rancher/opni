package targets

import (
	"context"
	"os/exec"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Runs golangci-lint with the project config and custom linters
func Lint(ctx context.Context) {
	_, tr := Tracer.Start(ctx, "target.lint")
	defer tr.End()

	mg.Deps(golangciLint, customLint)
}

func golangciLint() error {
	if lint, err := exec.LookPath("golangci-lint"); err == nil {
		return sh.RunV(lint, "run", "--fast")
	} else {
		return sh.RunV(mg.GoCmd(), "run", "github.com/golangci/golangci-lint/cmd/golangci-lint@latest", "run", "--fast")
	}
}

func customLint() error {
	mg.Deps(Build.Linter)
	return sh.Run("bin/lint", "./...")
}
