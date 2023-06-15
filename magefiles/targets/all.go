package targets

import (
	"context"

	"github.com/magefile/mage/mg"
)

// Runs all generate and build targets
func Default(ctx context.Context) {
	ctx, tr := Tracer.Start(ctx, "target.default")
	defer tr.End()

	mg.SerialCtxDeps(ctx, Generate.All, Build.All)
}

// Does nothing
func None() {}
