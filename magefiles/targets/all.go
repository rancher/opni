package targets

import (
	"context"

	"github.com/magefile/mage/mg"
)

var Default = All

func All(ctx context.Context) {
	ctx, tr := Tracer.Start(ctx, "target.all")
	defer tr.End()

	mg.SerialCtxDeps(ctx, Generate.All, Build.All)
}

var Aliases = map[string]any{
	"test": Test.Test,
}
