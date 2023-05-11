package main

import (
	"path/filepath"
	"strings"

	"github.com/kralicky/ragu"
	"github.com/kralicky/ragu/pkg/plugins/golang"
	"github.com/kralicky/ragu/pkg/plugins/golang/grpc"
	"github.com/kralicky/ragu/pkg/plugins/python"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/target"
	"github.com/rancher/opni/internal/codegen/cli"
)

func ProtobufGo() error {
	out, err := ragu.GenerateCode([]ragu.Generator{golang.Generator, grpc.Generator, cli.NewGenerator()},
		"internal/cli/*.proto",
		"pkg/**/*.proto",
		"plugins/**/*.proto",
	)
	if err != nil {
		return err
	}
	for _, file := range out {
		srcName := sourceProtoFilename(file.SourceRelPath)
		if shouldGenerate, _ := target.Path(file.SourceRelPath, srcName); !shouldGenerate {
			continue
		}

		if err := file.WriteToDisk(); err != nil {
			return err
		}
	}

	return nil
}

func sourceProtoFilename(path string) string {
	// /x/y/z.pb.go 				-> /x/y/z.proto
	// /x/y/z.pb.gw.go 			-> /x/y/z.proto
	// /x/y/z_grpc.pb.go 		-> /x/y/z.proto
	// /x/y/z.swagger.json 	-> /x/y/z.proto
	// /x/y/z.z.pb.go 			-> /x/y/z.z.proto

	for {
		switch ext := filepath.Ext(path); ext {
		case ".pb", ".gw", ".swagger", ".json", ".go":
			path = strings.TrimSuffix(path, ext)
		default:
			return strings.TrimSuffix(strings.TrimSuffix(path, "_grpc"), "_cli") + ".proto"
		}
	}
}

func ProtobufPython() error {
	out, err := ragu.GenerateCode([]ragu.Generator{python.Generator},
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

func Protobuf() {
	mg.Deps(ProtobufGo, ProtobufPython)
}
