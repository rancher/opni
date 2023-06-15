package targets

import (
	"context"

	"github.com/kralicky/ragu"
	"github.com/kralicky/ragu/pkg/plugins/golang"
	"github.com/kralicky/ragu/pkg/plugins/golang/grpc"
	"github.com/kralicky/ragu/pkg/plugins/python"
	"github.com/magefile/mage/mg"
	"github.com/rancher/opni/internal/codegen/cli"
)

// Generates Go protobuf code
func (Generate) ProtobufGo(ctx context.Context) error {
	_, tr := Tracer.Start(ctx, "target.generate.protobuf.go")
	defer tr.End()

	generators := []ragu.Generator{golang.Generator, grpc.Generator, cli.NewGenerator()}

	out, err := ragu.GenerateCode(
		generators,
		"internal/codegen/cli/*.proto",
		"pkg/**/*.proto",
		"plugins/**/*.proto",
	)
	if err != nil {
		return err
	}
	for _, file := range out {
		if err := file.WriteToDisk(); err != nil {
			return err
		}
	}

	return nil
}

// Generates Python protobuf code
func (Generate) ProtobufPython(ctx context.Context) error {
	_, tr := Tracer.Start(ctx, "target.generate.protobuf.python")
	defer tr.End()

	generators := []ragu.Generator{python.Generator}
	out, err := ragu.GenerateCode(generators,
		"aiops/**/*.proto",
	)
	if err != nil {
		return err
	}
	for _, file := range out {
		if err := file.WriteToDisk(); err != nil {
			return err
		}
	}
	return nil
}

// Generates all protobuf code
func (Generate) Protobuf(ctx context.Context) {
	ctx, tr := Tracer.Start(ctx, "target.generate.protobuf")
	defer tr.End()

	mg.CtxDeps(ctx, Generate.ProtobufGo, Generate.ProtobufPython)
}
