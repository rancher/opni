package targets

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kralicky/ragu"
	"github.com/kralicky/ragu/pkg/plugins/external"
	"github.com/kralicky/ragu/pkg/plugins/golang"
	"github.com/kralicky/ragu/pkg/plugins/golang/grpc"
	"github.com/kralicky/ragu/pkg/plugins/python"
	"github.com/magefile/mage/mg"
	"github.com/rancher/opni/internal/codegen"
	"github.com/rancher/opni/internal/codegen/cli"
	_ "go.opentelemetry.io/proto/otlp/metrics/v1"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/pluginpb"
)

// Generates Go protobuf code
func (Generate) ProtobufGo(ctx context.Context) error {
	mg.Deps(codegen.GenCortexConfig)
	_, tr := Tracer.Start(ctx, "target.generate.protobuf.go")
	defer tr.End()

	generators := []ragu.Generator{golang.Generator, grpc.Generator, cli.NewGenerator()}

	out, err := ragu.GenerateCode(
		generators,
		"internal/codegen/cli/*.proto",
		"internal/cortex/**/*.proto",
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

// Can be used to "bootstrap" the cli generator when modifying cli.proto
func (Generate) ProtobufCLI() error {
	out, err := ragu.GenerateCode([]ragu.Generator{golang.Generator, grpc.Generator, cli.NewGenerator()},
		"internal/codegen/cli/*.proto",
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

func (Generate) ProtobufTypescript() error {
	mg.Deps(Build.TypescriptServiceGenerator)
	destDir := "web/pkg/opni/generated"
	esGen, err := exec.LookPath("protoc-gen-es")
	if err != nil {
		return fmt.Errorf("cannot generate typescript code: %w", err)
	}

	targets := []string{
		"pkg/apis/management/v1/management.proto",
		"plugins/metrics/apis/cortexadmin/cortexadmin.proto",
		"plugins/metrics/apis/cortexops/cortexops.proto",
	}

	out, err := ragu.GenerateCode([]ragu.Generator{
		external.NewGenerator(esGen, external.GeneratorOptions{
			Opt: "target=ts,import_extension=none",
			CodeGeneratorRequestHook: func(req *pluginpb.CodeGeneratorRequest) {
				for _, f := range req.ProtoFile {
					if !slices.Contains(targets, f.GetName()) && !strings.HasPrefix(f.GetName(), "google/protobuf") {
						req.FileToGenerate = append(req.FileToGenerate, f.GetName())
					}
				}
			},
		}),
		external.NewGenerator([]string{"./web/service-generator/generate"}, external.GeneratorOptions{
			Opt: "target=ts",
		}),
	}, targets...)
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

	mg.CtxDeps(ctx, Generate.ProtobufGo, Generate.ProtobufPython)
}
