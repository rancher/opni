package targets

import (
	"context"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Runs golangci-lint with the project config and custom linters
func Lint(ctx context.Context) error {
	mg.CtxDeps(ctx, Build.Linter)
	_, tr := Tracer.Start(ctx, "target.lint")
	defer tr.End()

	return sh.RunV(mg.GoCmd(), "run", "github.com/golangci/golangci-lint/cmd/golangci-lint", "run", "-v", "--fast")
}
