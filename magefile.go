//go:build mage

package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
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
	// mage:import web
	"github.com/rancher/opni-monitoring/internal/mage/web"
	// mage:import test
	_ "github.com/rancher/opni-monitoring/internal/mage/test"
	// mage:import dev
	_ "github.com/rancher/opni-monitoring/internal/mage/dev"
)

var Default = All

func All() {
	// Only run webdist once, if web/dist/_nuxt doesn't exist yet
	if _, err := os.Stat("web/dist/_nuxt"); os.IsNotExist(err) {
		mg.Deps(web.Dist)
	}
	mg.Deps(build.Build)
}

func Generate() {
	mg.Deps(mockgen.Mockgen, protobuf.Protobuf, ControllerGen)
}

func ControllerGen() error {
	cmd := exec.Command(mg.GoCmd(), "run", "sigs.k8s.io/controller-tools/cmd/controller-gen",
		"crd:maxDescLen=0", "object", "paths=./pkg/sdk/api/...", "output:crd:artifacts:config=pkg/sdk/crd",
	)
	buf := new(bytes.Buffer)
	cmd.Stderr = buf
	cmd.Stdout = buf
	err := cmd.Run()
	if err != nil {
		if ex, ok := err.(*exec.ExitError); ok {
			if ex.ExitCode() != 1 {
				return err
			}
			bufStr := buf.String()
			lines := strings.Split(bufStr, "\n")
			for _, line := range lines {
				if strings.TrimSpace(line) == "" {
					continue
				}
				// ignore warnings that occur when running controller-gen on generated
				// protobuf code, but can be ignored
				if strings.Contains(line, "without JSON tag in type") ||
					strings.Contains(line, "not all generators ran successfully") ||
					strings.Contains(line, "for usage") ||
					strings.Contains(line, "exit status 1") {
					continue
				}
				fmt.Fprintln(os.Stderr, bufStr)
				return err
			}
		}
	}
	// copy pkg/sdk/crd/* to deploy/charts/opni-monitoring/crds
	if err := os.RemoveAll("deploy/charts/opni-monitoring/crds"); err != nil {
		return err
	}
	if err := os.MkdirAll("deploy/charts/opni-monitoring/crds", 0755); err != nil {
		return err
	}
	entries, err := os.ReadDir("pkg/sdk/crd")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := sh.Copy(path.Join("deploy/charts/opni-monitoring/crds", entry.Name()), path.Join("pkg/sdk/crd", entry.Name())); err != nil {
			return err
		}
	}
	return nil
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

func findProtos() []protobuf.Proto {
	var protos []protobuf.Proto
	filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".proto") {
			return nil
		}
		protos = append(protos, protobuf.Proto{
			Source:  path,
			DestDir: filepath.Dir(path),
		})
		return nil
	})
	return protos
}

func init() {
	build.Deps(Generate)
	docker.Deps(build.Build)
	test.Deps(testbin.Testbin, build.Build, HelmLint)

	k8sVersion := k8sModuleVersion()

	build.Config.ExtraTargets = map[string]string{
		"./internal/cmd/testenv": "bin/testenv",
		"./plugins/example":      "bin/plugin_example",
		"./plugins/cortex":       "bin/plugin_cortex",
		"./plugins/logging":      "bin/plugin_logging",
	}
	build.Config.ExtraEnv = map[string]string{
		"GOOS": "linux",
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
		{
			Source: "pkg/plugins/apis/capability/plugin.go",
			Dest:   "pkg/test/mock/capability/capability.go",
			Types:  []string{"Backend"},
		},
	}
	protobuf.Config.Protos = findProtos()
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
