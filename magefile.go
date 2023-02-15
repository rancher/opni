//go:build mage

package main

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/samber/lo"

	"github.com/kralicky/ragu"
	_ "github.com/kralicky/ragu/compat"
	"github.com/kralicky/ragu/pkg/plugins/python"

	// mage:import
	"github.com/kralicky/spellbook/mockgen"
	// mage:import
	"github.com/kralicky/spellbook/testbin"
	// mage:import dev
	_ "github.com/rancher/opni/internal/mage/dev"
	// mage:import charts
	charts "github.com/rancher/charts-build-scripts/pkg/actions"

	// mage:import test
	"github.com/rancher/opni/internal/mage/test"
)

var Default = All

func All() {
	mg.SerialDeps(Generate, Build)
}

func goBuild(args ...string) error {
	tag, _ := os.LookupEnv("BUILD_VERSION")
	tag = strings.TrimSpace(tag)

	version := "unversioned"
	if tag != "" {
		version = tag
	}

	defaultArgs := []string{
		"build",
		"-ldflags", fmt.Sprintf("-w -s -X github.com/rancher/opni/pkg/versions.Version=%s -X github.com/rancher/opni/pkg/versions.BuildTime=%s", version, time.Now().UTC().Format(time.RFC3339)),
		"-trimpath",
		"-o", "./bin/",
	}

	// disable vcs stamping inside git worktrees if the linked git directory doesn't exist
	dotGit, err := os.Stat(".git")
	if err != nil {
		return err
	}
	if !dotGit.IsDir() {
		fmt.Println("disabling vcs stamping inside worktree")
		defaultArgs = append(defaultArgs, "-buildvcs=false")
	}

	return sh.RunWith(map[string]string{
		"CGO_ENABLED": "0",
	}, mg.GoCmd(), append(defaultArgs, args...)...)
}

func Build() error {
	if err := goBuild("./cmd/...", "./internal/cmd/...", "./plugins/..."); err != nil {
		return err
	}

	// create bin/plugins if it doesn't exist
	if _, err := os.Stat("bin/plugins"); os.IsNotExist(err) {
		if err := os.Mkdir("bin/plugins", 0755); err != nil {
			return err
		}
	}

	// move plugins to bin/plugins
	entries, err := os.ReadDir("./plugins")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// add plugin_ prefix
		pluginName := entry.Name()
		if _, err := os.Stat(filepath.Join("bin", pluginName)); err == nil {
			if err := os.Rename(filepath.Join("bin", pluginName), filepath.Join("bin", "plugins", "plugin_"+pluginName)); err != nil {
				return err
			}
		}
	}
	return nil
}

func Generate() {
	mg.SerialDeps(Protobuf, mockgen.Mockgen, ControllerGen)
}

func GenerateCRD() {
	mg.SerialDeps(CRDGen, ReplaceCRDText)
}

func Test() {
	mg.Deps(TestClean, test.Test)
}

func TestClean() error {
	// find and remove all test binaries and coverage files
	return sh.Run("git", "clean", "-xf", "--", "**/*.test", "**/cover-*.out")
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

	expr := `del(.. | select(has("description")).description) | .. style="flow"`

	e1 := lo.Async(func() error {
		return sh.Run(mg.GoCmd(), "run", "github.com/mikefarah/yq/v4", "-i", expr, "./packages/opni/opni/charts/crds/crds.yaml")
	})
	e2 := lo.Async(func() error {
		return sh.Run(mg.GoCmd(), "run", "github.com/mikefarah/yq/v4", "-i", expr, "./packages/opni-agent/opni-agent/charts/crds/crds.yaml")
	})

	if err := <-e1; err != nil {
		return err
	}
	if err := <-e2; err != nil {
		return err
	}

	// prepend "---" to each file, otherwise kubernetes will think it's json
	for _, f := range []string{"./packages/opni/opni/charts/crds/crds.yaml", "./packages/opni-agent/opni-agent/charts/crds/crds.yaml"} {
		if err := prependDocumentSeparator(f); err != nil {
			return err
		}
	}

	return nil
}

func prependDocumentSeparator(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	i, err := f.Stat()
	if err != nil {
		return err
	}

	buf := make([]byte, i.Size()+4)
	copy(buf[:4], "---\n")

	_, err = f.Read(buf[4:])
	if err != nil {
		return err
	}

	f.Seek(0, 0)
	f.Truncate(0)
	_, err = f.Write(buf)
	if err != nil {
		return err
	}

	return nil
}

func ReplaceCRDText() error {
	files := []string{
		"./packages/opni/opni/charts/crds/crds.yaml",
		"./packages/opni-agent/opni-agent/charts/crds/crds.yaml",
	}

	for _, file := range files {
		input, err := ioutil.ReadFile(file)
		if err != nil {
			return err
		}

		firstReplace := bytes.Replace(input, []byte("replace-me/opni-serving-cert"), []byte(`"replace-me/opni-serving-cert"`), -1)
		output := bytes.Replace(firstReplace, []byte("replace-me"), []byte("{{ .Release.Namespace }}"), -1)

		if err := ioutil.WriteFile(file, output, 0644); err != nil {
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

func getJetstreamVersion(binary string) string {
	output, err := sh.Output(binary, "--version")
	if err != nil {
		panic(fmt.Sprintf("failed to query version for %s: %v", binary, err))
	}
	fmt.Println(output)
	return strings.Split(output, " ")[1][1:]
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
	test.Deps = append(test.Deps, testbin.Testbin, Build) // TODO: fix this with magefiles directory

	k8sVersion := k8sModuleVersion()

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
			Source: "pkg/apis/capability/v1/capability_grpc.pb.go",
			Dest:   "pkg/test/mock/capability/backend.go",
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
		{
			Name:       "alertmanager",
			Version:    "0.25.0",
			URL:        "https://github.com/prometheus/alertmanager/releases/download/v{{.Version}}/alertmanager-{{.Version}}.{{.GOOS}}-{{.GOARCH}}.tar.gz",
			GetVersion: getVersion,
		},
		{
			Name:       "amtool",
			Version:    "0.24.0",
			URL:        "https://github.com/prometheus/alertmanager/releases/download/v{{.Version}}/alertmanager-{{.Version}}.{{.GOOS}}-{{.GOARCH}}.tar.gz",
			GetVersion: getVersion,
		},
		{
			Name:       "nats-server",
			Version:    "2.9.3",
			URL:        "https://github.com/nats-io/nats-server/releases/download/v{{.Version}}/nats-server-v{{.Version}}-{{.GOOS}}-{{.GOARCH}}.tar.gz",
			GetVersion: getJetstreamVersion,
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
			testbin.Binary{
				Name:       "node_exporter",
				Version:    "1.4.0",
				URL:        "https://github.com/prometheus/node_exporter/releases/download/v{{.Version}}/node_exporter-{{.Version}}.{{.GOOS}}-{{.GOARCH}}.tar.gz",
				GetVersion: func(string) string { return "1.4.0" },
			},
		)
	}
}
func ProtobufGo() error {
	out, err := ragu.GenerateCode(ragu.DefaultGenerators(),
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

func Minimal() error {
	if err := goBuild(
		"-tags",
		"noagentv1,noplugins,nohooks,norealtime,nomanager,nocortex,nodebug,noevents,nogateway,noetcd,noscheme_thirdparty,noalertmanager,nomsgpack",
		"./cmd/opni",
	); err != nil {
		return err
	}

	// create bin/plugins if it doesn't exist
	if _, err := os.Stat("bin/plugins"); os.IsNotExist(err) {
		if err := os.Mkdir("bin/plugins", 0755); err != nil {
			return err
		}
	}

	return nil
}

func Charts() {
	mg.SerialDeps(All, CRDGen, func() {
		charts.Charts("opni")
	}, func() {
		charts.Charts("opni-agent")
	})
}

func ChartsV(version string) error {
	if strings.HasPrefix(version, "v") {
		version = version[1:]
	}
	fmt.Println("Patching chart version to " + version)
	for _, packageSpec := range []string{
		"./packages/opni-agent/opni-agent/package.yaml",
		"./packages/opni-agent/opni-agent/charts/Chart.yaml",
		"./packages/opni/opni/package.yaml",
		"./packages/opni/opni/charts/Chart.yaml",
	} {
		contents, err := os.ReadFile(packageSpec)
		if err != nil {
			return err
		}
		contents = regexp.MustCompile(`(?m)^(app)?([vV])ersion: .*$`).
			ReplaceAll(contents, []byte(fmt.Sprintf("${1}${2}ersion: %s", version)))

		if err := os.WriteFile(packageSpec, contents, 0644); err != nil {
			return err
		}
		fmt.Printf("%s => %s\n", packageSpec, version)
	}
	Charts()
	return nil
}
