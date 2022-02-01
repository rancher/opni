//go:build mage

package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/ttacon/chalk"

	"github.com/kralicky/opni-monitoring/pkg/logger"
	pkgtest "github.com/kralicky/opni-monitoring/pkg/test"

	// mage:import
	"github.com/kralicky/spellbook/build"
	// mage:import
	test "github.com/kralicky/spellbook/test/ginkgo"
	// mage:import
	"github.com/kralicky/spellbook/docker"
	// mage:import
	"github.com/kralicky/spellbook/mockgen"
	// mage:import
	protobuf "github.com/kralicky/spellbook/protobuf/ragu"
	// mage:import
	"github.com/kralicky/spellbook/testbin"
)

var Default = All

func All() {
	mg.SerialDeps(build.Build)
}

func Generate() {
	mg.Deps(mockgen.Mockgen, protobuf.Protobuf)
}

// "prometheus, version x.y.z"
// "etcd Version: x.y.z"
// "Cortex, version x.y.z"
func getVersion(binary string) string {
	version, err := sh.Output(binary, "--version")
	if err != nil {
		panic(fmt.Sprintf("failed to query version for %s: %v", binary, err))
	}
	return strings.Split(strings.Split(version, "\n")[0], " ")[2]
}

func init() {
	build.Deps(Generate)
	docker.Deps(build.Build)
	test.Deps(testbin.Testbin)

	mockgen.Config.Mocks = []mockgen.Mock{
		{
			Source: "pkg/rbac/rbac.go",
			Dest:   "pkg/test/mock/rbac/rbac.go",
			Types:  []string{"Provider"},
		},
		{
			Source: "pkg/storage/stores.go",
			Dest:   "pkg/test/mock/storage/stores.go",
			Types:  []string{"TokenStore", "TenantStore"},
		},
		{
			Source: "pkg/ident/ident.go",
			Dest:   "pkg/test/mock/ident/ident.go",
			Types:  []string{"Provider"},
		},
	}
	protobuf.Config.Protos = []protobuf.Proto{
		{
			Source:  "pkg/core/core.proto",
			DestDir: "pkg/core",
		},
		{
			Source:  "pkg/management/management.proto",
			DestDir: "pkg/management",
		},
	}
	// protobuf.Config.Options = []ragu.GenerateCodeOption{
	// 	ragu.ExperimentalHideEmptyMessages(),
	// }
	docker.Config.Tag = "kralicky/opni-monitoring"
	ext := ".tar.gz"
	if runtime.GOOS == "darwin" {
		ext = ".zip"
	}
	testbin.Config.Binaries = []testbin.Binary{
		{
			Name:       "etcd",
			Version:    "3.5.1",
			URL:        "https://storage.googleapis.com/etcd/v{{.Version}}/etcd-v{{.Version}}-{{.GOOS}}-{{.GOARCH}}" + ext,
			GetVersion: getVersion,
		},
		{
			Name:       "prometheus",
			Version:    "2.32.1",
			URL:        "https://github.com/prometheus/prometheus/releases/download/v{{.Version}}/prometheus-{{.Version}}.{{.GOOS}}-{{.GOARCH}}.tar.gz",
			GetVersion: getVersion,
		},
		{
			Name:       "cortex",
			Version:    "1.11.0",
			URL:        "https://github.com/cortexproject/cortex/releases/download/v{{.Version}}/cortex-{{.GOOS}}-{{.GOARCH}}",
			GetVersion: getVersion,
		},
	}
}

func TestEnv() {
	fmt.Println("Starting test environment")
	environment := &pkgtest.Environment{
		TestBin: "testbin/bin",
		Logger:  logger.New().Named("test"),
	}
	if err := environment.Start(); err != nil {
		panic(err)
	}
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	fmt.Println(chalk.Blue.Color("Press Ctrl+C to stop test environment"))
	<-c
	fmt.Println("\nStopping test environment")
	if err := environment.Stop(); err != nil {
		panic(err)
	}
}
