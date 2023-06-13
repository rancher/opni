package targets

import (
	"context"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

func Lint(ctx context.Context) error {
	mg.CtxDeps(ctx, Build.Linter)
	_, tr := Tracer.Start(ctx, "target.lint")
	defer tr.End()

	return sh.Run(mg.GoCmd(), "run", "github.com/golangci/golangci-lint/cmd/golangci-lint", "run", "-v", "--fast")
}
