package targets

import (
	"context"

	"github.com/magefile/mage/mg"
)

// Builds the test environment binary
func (Build) Testenv(ctx context.Context) error {
	_, tr := Tracer.Start(ctx, "target.build.testenv")
	defer tr.End()
	mg.CtxDeps(ctx, Build.Archives)

	return buildMainPackage(buildOpts{
		Path:   "./internal/cmd/testenv",
		Output: "bin/testenv",
		Tags:   []string{"nomsgpack"},
		Debug:  true,
	})
}
