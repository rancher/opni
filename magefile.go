//go:build mage

package main

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"

	"github.com/kralicky/ragu/pkg/plugins/golang/gateway"
	"github.com/kralicky/ragu/pkg/plugins/python"
	"github.com/kralicky/ragu/pkg/ragu"

	//mage:import
	"github.com/kralicky/spellbook/build"

	// mage:import
	"github.com/kralicky/spellbook/mockgen"
	// mage:import
	"github.com/kralicky/spellbook/testbin"
	// mage:import dev
	_ "github.com/rancher/opni/internal/mage/dev"

	// mage:import charts
	_ "github.com/rancher/charts-build-scripts/pkg/actions"
	// mage:import test
	test "github.com/rancher/opni/internal/mage/test"
)

var Default = All

func All() {
	mg.SerialDeps(Generate, build.Build)
}

func Generate() {
	mg.SerialDeps(Protobuf, mockgen.Mockgen, ControllerGen)
}

func Test() {
	mg.Deps(test.Test)
}

func ControllerGen() error {
	cmd := exec.Command(mg.GoCmd(), "run", "sigs.k8s.io/controller-tools/cmd/controller-gen",
		"crd:maxDescLen=0", "rbac:roleName=manager-role", "webhook", "object", "paths=./apis/...", "output:crd:artifacts:config=config/crd/bases",
	)
	buf := new(bytes.Buffer)
	cmd.Stderr = buf
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
					strings.Contains(line, "exit status 1") ||
					strings.HasPrefix(line, "go:") {
					continue
				}
				fmt.Fprintln(os.Stderr, "[controller-gen] "+line)
				return err
			}
		}
	}

	return nil
}

func CRDGen() error {
	var commands []*exec.Cmd
	commands = append(commands, exec.Command(mg.GoCmd(), "run", "sigs.k8s.io/kustomize/kustomize/v4",
		"build", "./config/chart-crds", "-o", "./packages/opni/opni/charts/crds/crds.yaml",
	))
	commands = append(commands, exec.Command(mg.GoCmd(), "run", "sigs.k8s.io/kustomize/kustomize/v4",
		"build", "./config/agent-chart-crds", "-o", "./packages/opni-agent/opni-agent/charts/crds/crds.yaml",
	))
	for _, cmd := range commands {
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
					fmt.Fprintln(os.Stderr, line)
					return err
				}
			}
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

func init() {
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
		//FIXME: github.com/golang/google/mock doesn't fully work with generic interfaces,
		// For now need to manually fix some of the generated code
		// Uncomment when https://github.com/golang/mock/issues/658 is fixed
		// {
		// 	Source: "pkg/util/notifier/types.go",
		// 	Dest:   "pkg/test/mock/notifier/notifier.go",
		// 	Types:  []string{"UpdateNotifier", "Finder", "Clonable"},
		// },
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
			Version:    "2.35.0",
			URL:        "https://github.com/prometheus/prometheus/releases/download/v{{.Version}}/prometheus-{{.Version}}.{{.GOOS}}-{{.GOARCH}}.tar.gz",
			GetVersion: getVersion,
		},
		{
			Name:       "promtool",
			Version:    "2.35.0",
			URL:        "https://github.com/prometheus/prometheus/releases/download/v{{.Version}}/prometheus-{{.Version}}.{{.GOOS}}-{{.GOARCH}}.tar.gz",
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

func ProtobufGo() error {
	out, err := ragu.GenerateCode(append(ragu.DefaultGenerators(), gateway.Generator),
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
