package targets

import (
	"context"
	"os"
	"slices"

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

func takeArgv(arg0 string) (rest []string) {
	idx := slices.Index(os.Args, arg0)
	rest, os.Args = os.Args[idx:], os.Args[:idx]
	return
}
