package targets

import (
	"context"

	"github.com/magefile/mage/mg"
)

type Generate mg.Namespace

// Runs all generators (protobuf, mocks, controllers)
func (Generate) All(ctx context.Context) {
	ctx, tr := Tracer.Start(ctx, "target.generate")
	defer tr.End()

	mg.CtxDeps(ctx, Generate.Protobuf, Generate.Mocks, Generate.GenerateCRD, Generate.Controllers)
}
