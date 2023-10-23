package targets

import (
	"context"
	"fmt"

	"github.com/magefile/mage/mg"
)

// Builds the opni binary
func (Build) Opni(ctx context.Context) error {
	mg.CtxDeps(ctx, Build.Archives)

	_, tr := Tracer.Start(ctx, "target.build.opni")
	defer tr.End()

	return buildMainPackage(buildOpts{
		Path:   "./cmd/opni",
		Output: "bin/opni",
		Tags:   []string{"nomsgpack"},
	})
}

// Builds the opni-minimal binary
func (Build) OpniMinimal(ctx context.Context) error {
	_, tr := Tracer.Start(ctx, "target.build.opni-minimal")
	defer tr.End()

	return buildMainPackage(buildOpts{
		Path:   "./cmd/opni",
		Output: "bin/opni-minimal",
		Tags:   []string{"nomsgpack", "minimal"},
	})
}

// Builds the opni release CLI binary, requires version as input
func (Build) OpniReleaseCLI(ctx context.Context, fileSuffix string) error {
	mg.CtxDeps(ctx, Build.Archives)

	_, tr := Tracer.Start(ctx, "target.build.opni-cli-release")
	defer tr.End()

	return buildMainPackage(buildOpts{
		Path:     "./cmd/opni",
		Output:   fmt.Sprintf("bin/opni_%s", fileSuffix),
		Tags:     []string{"nomsgpack", "cli"},
		Compress: true,
	})
}
