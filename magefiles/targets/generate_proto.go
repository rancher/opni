package targets

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kralicky/ragu"
	"github.com/kralicky/ragu/pkg/plugins/external"
	"github.com/kralicky/ragu/pkg/plugins/golang"
	"github.com/kralicky/ragu/pkg/plugins/golang/grpc"
	"github.com/kralicky/ragu/pkg/plugins/python"
	"github.com/magefile/mage/mg"
	"github.com/rancher/opni/internal/codegen"
	"github.com/rancher/opni/internal/codegen/cli"
	"github.com/rancher/opni/internal/codegen/templating"
	_ "go.opentelemetry.io/proto/otlp/metrics/v1"
)

// Generates Go protobuf code
func (Generate) ProtobufGo(ctx context.Context) error {
	mg.Deps(codegen.GenCortexConfig) // codegen.GenAlertManagerReceiver)
	_, tr := Tracer.Start(ctx, "target.generate.protobuf.go")
	defer tr.End()

	grpc.SetRequireUnimplemented(false)
	generators := []ragu.Generator{templating.CommentRenderer{}, golang.Generator, grpc.Generator, cli.NewGenerator()}

	out, err := ragu.GenerateCode(
		generators,
		[]string{
			"internal/codegen/cli",
			"internal/cortex",
			"internal/alertmanager",
			"pkg",
			"plugins",
		},
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

// Can be used to "bootstrap" the cli generator when modifying cli.proto
func (Generate) ProtobufCLI() error {
	out, err := ragu.GenerateCode([]ragu.Generator{golang.Generator, grpc.Generator, cli.NewGenerator()},
		[]string{"internal/codegen/cli"},
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
	out, err := ragu.GenerateCode(generators, []string{"aiops"})
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

func (Generate) ProtobufTypescript() error {
	mg.Deps(Build.TypescriptServiceGenerator)
	destDir := "web/pkg/opni/generated"

	searchDirs := []string{
		"pkg/apis/management/v1",
		"plugins/metrics/apis/cortexadmin",
		"plugins/metrics/apis/cortexops",
	}

	out, err := ragu.GenerateCode([]ragu.Generator{
		external.NewGenerator("./web/service-generator/node_modules/.bin/protoc-gen-es", external.GeneratorOptions{
			Opt: "target=ts,import_extension=none,ts_nocheck=false",
		}),
		external.NewGenerator([]string{"./web/service-generator/generate"}, external.GeneratorOptions{
			Opt: "target=ts,import_extension=none,ts_nocheck=false",
		}),
	}, searchDirs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error generating typescript code: %v\n", err)
		return err
	}

	for _, file := range out {
		file.SourceRelPath = filepath.Join(destDir, file.Package, file.Name)
		os.MkdirAll(filepath.Dir(file.SourceRelPath), 0755)
		if err := file.WriteToDisk(); err != nil {
			return fmt.Errorf("error writing file %s: %w", file.SourceRelPath, err)
		}
	}
	return nil
}

// Generates all protobuf code
func (Generate) Protobuf(ctx context.Context) {
	ctx, tr := Tracer.Start(ctx, "target.generate.protobuf")
	defer tr.End()

	_, err := exec.LookPath("yarn")
	if err == nil {
		mg.CtxDeps(ctx, Generate.ProtobufGo, Generate.ProtobufPython)
	} else {
		mg.CtxDeps(ctx, Generate.ProtobufGo, Generate.ProtobufPython)
	}
}
