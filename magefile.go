//go:build mage

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"

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
	mg.Deps(build.Build)
}

func Generate() {
	mg.Deps(mockgen.Mockgen, protobuf.Protobuf)
}

func HelmLint() error {
	chartDirs, err := os.ReadDir("deploy/charts")
	if err != nil {
		return err
	}
	for _, chartDir := range chartDirs {
		if !chartDir.IsDir() {
			continue
		}
		if err := sh.Run("helm", "lint", path.Join("deploy/charts", chartDir.Name())); err != nil {
			return err
		}
	}
	return nil
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

func getKubeVersion(binary string) string {
	version, err := sh.Output(binary, "--version")
	if err != nil {
		panic(fmt.Sprintf("failed to query version for %s: %v", binary, err))
	}
	return strings.TrimSpace(strings.TrimPrefix(version, "Kubernetes v"))
}

func k8sModuleVersion() string {
	buf := &bytes.Buffer{}
	cmd := exec.Command(mg.GoCmd(), "list", "-m", "k8s.io/api")
	cmd.Stdout = buf
	if err := cmd.Run(); err != nil {
		panic(fmt.Sprintf("failed to query k8s.io/api module version: %v\n", err))
	}
	out := buf.String()
	return strings.TrimSpace(strings.Replace(strings.Split(out, " ")[1], "v0", "1", 1))
}

func init() {
	build.Deps(Generate)
	docker.Deps(build.Build)
	test.Deps(testbin.Testbin, build.Build, HelmLint)

	k8sVersion := k8sModuleVersion()

	build.Config.ExtraTargets = map[string]string{
		"./plugins/example": "bin/plugin_example",
	}
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
		{
			Source: "pkg/plugins/apis/apiextensions/apiextensions_grpc.pb.go",
			Dest:   "pkg/test/mock/apiextensions/apiextensions.go",
			Types:  []string{"ManagementAPIExtensionServer"},
		},
		{
			Source: "pkg/test/testdata/plugins/ext/ext_grpc.pb.go",
			Dest:   "pkg/test/mock/ext/ext.go",
			Types:  []string{"ManagementAPIExtensionServer"},
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
		{
			Source:  "pkg/plugins/apis/apiextensions/apiextensions.proto",
			DestDir: "pkg/plugins/apis/apiextensions",
		},
		{
			Source:  "pkg/plugins/apis/system/system.proto",
			DestDir: "pkg/plugins/apis/system",
		},
		{
			Source:  "plugins/example/pkg/example.proto",
			DestDir: "plugins/example/pkg",
		},
		{
			Source:  "pkg/test/testdata/plugins/ext/ext.proto",
			DestDir: "pkg/test/testdata/plugins/ext",
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
	if runtime.GOOS == "linux" {
		testbin.Config.Binaries = append(testbin.Config.Binaries,
			testbin.Binary{
				Name:       "kube-apiserver",
				Version:    k8sVersion,
				URL:        "https://dl.k8s.io/v{{.Version}}/bin/linux/{{.GOARCH}}/kube-apiserver",
				GetVersion: getKubeVersion,
			},
		)
	}
}

func TestEnv() {
	mg.Deps(build.Build)
	sh.RunV("bin/testenv")
}
