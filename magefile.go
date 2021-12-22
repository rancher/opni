//go:build mage

package main

import (
	"fmt"
	"os"

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
	return sh.RunWith(map[string]string{
		"CGO_ENABLED": "0",
	}, mg.GoCmd(), "build", "-ldflags", "-w -s", "-o", "bin/opni-gateway", "./cmd/opni-gateway")
}

func Test() error {
	return nil
}

func Run() error {
	mg.Deps(Build)
	return sh.RunV("./bin/opni-gateway", runArgs...)
}

func Docker() error {
	mg.Deps(Build)
	return sh.RunWithV(map[string]string{
		"DOCKER_BUILDKIT": "1",
	}, "docker", "build", "-t", "opni-gateway", ".")
}
