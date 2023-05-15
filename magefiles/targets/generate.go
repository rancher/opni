package targets

import (
	"context"

	"github.com/magefile/mage/mg"
)

type Generate mg.Namespace

func (Generate) All(ctx context.Context) {
	ctx, tr := Tracer.Start(ctx, "target.generate")
	defer tr.End()

	mg.CtxDeps(ctx, Generate.Protobuf, Generate.Mocks, Generate.Controllers)
}
