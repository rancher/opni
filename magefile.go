//go:build mage

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kralicky/ragu/pkg/ragu"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

var Default = All
var runArgs []string

func init() {
	if len(os.Args) > 1 && os.Args[1] == "run" {
		idx := 0
		for i, arg := range os.Args {
			if arg == "--" {
				idx = i
				break
			}
		}
		if idx == 0 {
			fmt.Println("usage: mage run -- <args>")
			os.Exit(1)
		}
		runArgs = os.Args[idx+1:]
		os.Args = os.Args[:idx]
	}
}

func All() {
	mg.SerialDeps(
		Build,
		Test,
	)
}

func Build() error {
	mg.Deps(Generate)
	return sh.RunWith(map[string]string{
		"CGO_ENABLED": "0",
	}, mg.GoCmd(), "build", "-ldflags", "-w -s", "-o", "bin/opnim", "./cmd/opnim")
}

func Test() error {
	return nil
}

func Run() error {
	mg.Deps(Build)
	return sh.RunV("./bin/opnim", runArgs...)
}

func Docker() error {
	mg.Deps(Build)
	return sh.RunWithV(map[string]string{
		"DOCKER_BUILDKIT": "1",
	}, "docker", "build", "-t", "kralicky/opni-monitoring", ".")
}

func Generate() error {
	protos, err := ragu.GenerateCode("pkg/management/management.proto", true)
	if err != nil {
		return err
	}
	for _, f := range protos {
		path := filepath.Join("pkg/management", f.GetName())
		if info, err := os.Stat(path); err == nil {
			if info.Mode()&0200 == 0 {
				if err := os.Chmod(path, 0644); err != nil {
					return err
				}
			}
		}
		if err := os.WriteFile(path, []byte(f.GetContent()), 0444); err != nil {
			return err
		}
		if err := os.Chmod(path, 0444); err != nil {
			return err
		}
	}
	return nil
}
