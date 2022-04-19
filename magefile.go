//go:build mage

package main

import (
	"bytes"
	"errors"
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
	"github.com/rancher/opni/internal/mage/web"
	// mage:import test
	_ "github.com/rancher/opni/internal/mage/test"
	// mage:import dev
	_ "github.com/rancher/opni/internal/mage/dev"
)

var Default = All

func All() {
	// Only run webdist once, if web/dist/_nuxt doesn't exist yet
	if _, err := os.Stat("web/dist/_nuxt"); os.IsNotExist(err) {
		mg.Deps(web.Dist)
	}
	mg.SerialDeps(Generate, build.Build)
}

func Generate() {
	mg.SerialDeps(protobuf.Protobuf, mockgen.Mockgen, ControllerGen)
}

func ControllerGen() error {
	cmd := exec.Command(mg.GoCmd(), "run", "sigs.k8s.io/controller-tools/cmd/controller-gen",
		"crd:maxDescLen=0", "rbac:roleName=manager-role", "webhook", "object", "paths=./...", "output:crd:artifacts:config=config/crd/bases",
	)
	buf := new(bytes.Buffer)
	cmd.Stderr = buf
	cmd.Stdout = buf
	err := cmd.Run()
	if err != nil {
		if ex, ok := err.(*exec.ExitError); ok {
			if ex.ExitCode() != 1 {
				return errors.New(buf.String())
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
				fmt.Fprintln(os.Stderr, line)
				return err
			}
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

func E2e() error {
	regName := "registry.local"
	regPort := 5000
	mg.Deps(testbin.Testbin)
	output, err := sh.Output("kubectl",
		strings.Fields(`get nodes -o go-template --template='{{range .items}}{{printf "%s\n" .metadata.name}}{{end}}`)...)
	if err != nil {
		return err
	}
	nodes := strings.Fields(output)
	for _, node := range nodes {
		if err := sh.Run("kubectl", "annotate", "node", node,
			fmt.Sprintf("tilt.dev/registry=k3dsvc:%d", regPort),
			fmt.Sprintf("tilt.dev/registry-from-cluster=%s:%d", regName, regPort),
		); err != nil {
			return err
		}
	}
	return sh.Run("tilt", "ci", "e2e-tests-prod")
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

func getKubectlVersion(binary string) string {
	version, err := sh.Output(binary, "version", "--client", "--short")
	if err != nil {
		panic(fmt.Sprintf("failed to query version for %s: %v", binary, err))
	}
	return strings.TrimSpace(strings.TrimPrefix(version, "Client Version: v"))
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
	docker.Deps(build.Build)
	test.Deps(testbin.Testbin, build.Build)

	labelFilter := "!e2e"
	if filter, ok := os.LookupEnv("GINKGO_LABEL_FILTER"); ok {
		labelFilter = filter
	}
	test.Config.GinkgoArgs = append(test.Config.GinkgoArgs, "--label-filter="+labelFilter)

	k8sVersion := k8sModuleVersion()

	extraTargets := map[string]string{}
	// find plugins
	entries, err := os.ReadDir("./plugins")
	if err != nil {
		panic(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		extraTargets["./plugins/"+entry.Name()] = "bin/plugins/plugin_" + entry.Name()
	}
	// find (optional) internal cmds
	if entries, err = os.ReadDir("./internal/cmd"); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			extraTargets["./internal/cmd/"+entry.Name()] = "bin/" + entry.Name()
		}
	}

	// get version info
	dirty := true
	if output, err := sh.Output("git", "status", "--porcelain", "--untracked-files=no"); err != nil {
		panic(err)
	} else if strings.TrimSpace(output) == "" {
		dirty = false
	}
	var tag string
	if droneTag, ok := os.LookupEnv("DRONE_TAG"); ok {
		tag = droneTag
	} else {
		tag, err = sh.Output("git", "tag", "-l", "--points-at", "HEAD")
		if err != nil {
			panic(err)
		}
	}
	tag = strings.TrimSpace(tag)

	version := "dev"
	if !dirty && tag != "" {
		version = tag
	}

	build.Config.LDFlags = append(build.Config.LDFlags, "-X", "github.com/rancher/opni/pkg/util.Version="+version)
	build.Config.ExtraTargets = extraTargets

	mockgen.Config.Mocks = []mockgen.Mock{
		{
			Source: "pkg/rbac/rbac.go",
			Dest:   "pkg/test/mock/rbac/rbac.go",
			Types:  []string{"Provider"},
		},
		{
			Source: "pkg/rules/types.go",
			Dest:   "pkg/test/mock/rules/rules.go",
			Types:  []string{"RuleFinder"},
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
			Dest:   "pkg/test/mock/capability/backend.go",
			Types:  []string{"Backend"},
		},
		{
			Source: "pkg/plugins/apis/capability/capability_grpc.pb.go",
			Dest:   "pkg/test/mock/capability/backend_client.go",
			Types:  []string{"BackendClient"},
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
			testbin.Binary{
				Name:       "kube-controller-manager",
				Version:    k8sVersion,
				URL:        "https://dl.k8s.io/v{{.Version}}/bin/linux/{{.GOARCH}}/kube-controller-manager",
				GetVersion: getKubeVersion,
			},
			testbin.Binary{
				Name:       "kubectl",
				Version:    k8sVersion,
				URL:        "https://dl.k8s.io/v{{.Version}}/bin/linux/{{.GOARCH}}/kubectl",
				GetVersion: getKubectlVersion,
			},
		)
	}
}
