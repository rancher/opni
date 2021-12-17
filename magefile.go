//go:build mage

package main

import (
	"os"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

var Default = All

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
	return sh.RunV("./bin/opni-gateway", os.Args[2:]...)
}

func Docker() error {
	mg.Deps(Build)
	return sh.RunWithV(map[string]string{
		"DOCKER_BUILDKIT": "1",
	}, "docker", "build", "-t", "opni-gateway", ".")
}
