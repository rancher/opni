package targets

import (
	"context"

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
	return sh.Run("golangci-lint", "run", "-v", "--fast")
}

func customLint() error {
	mg.Deps(Build.Linter)
	return sh.Run("bin/lint", "./...")
}
