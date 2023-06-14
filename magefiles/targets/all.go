package targets

import (
	"context"

	"github.com/magefile/mage/mg"
)

func Default(ctx context.Context) {
	ctx, tr := Tracer.Start(ctx, "target.default")
	defer tr.End()

	mg.SerialCtxDeps(ctx, Generate.All, Build.All)
}

func None() {}
