//go:build mage

package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/schollz/progressbar/v3"

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
	// Only run webdist once, if web/dist/_nuxt doesn't exist yet
	if _, err := os.Stat("web/dist/_nuxt"); os.IsNotExist(err) {
		mg.Deps(WebDist)
	}
	mg.Deps(build.Build)
}

func Generate() {
	mg.Deps(mockgen.Mockgen, protobuf.Protobuf, ControllerGen)
}

func ControllerGen() error {
	cmd := exec.Command(mg.GoCmd(), "run", "sigs.k8s.io/controller-tools/cmd/controller-gen",
		"crd", "object", "paths=./pkg/sdk/api/...", "output:crd:artifacts:config=pkg/sdk/crd",
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

func init() {
	build.Deps(Generate)
	docker.Deps(build.Build)
	test.Deps(testbin.Testbin, build.Build, HelmLint)

	k8sVersion := k8sModuleVersion()

	build.Config.ExtraTargets = map[string]string{
		"./internal/cmd/testenv": "bin/testenv",
		"./plugins/example":      "bin/plugin_example",
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

type webAssetFile struct {
	Path string
	Data []byte
}

func WebDist() error {
	err := sh.RunWith(map[string]string{
		"DOCKER_BUILDKIT": "1",
	}, "docker", "build", "-t", "opni-monitoring-ui-build", "-f", "Dockerfile.ui", ".")
	if err != nil {
		return err
	}
	pwd, err := os.Getwd()
	if err != nil {
		return err
	}
	curUser, err := user.Current()
	if err != nil {
		return err
	}
	err = sh.Run("docker", "run", "-t", "--rm", "-v",
		filepath.Join(pwd, "web/dist")+":/dist",
		"opni-monitoring-ui-build", fmt.Sprintf("%s:%s", curUser.Uid, curUser.Gid))
	if err != nil {
		return err
	}
	count := 0
	uncompressedSize := int64(0)
	compressedSize := int64(0)
	workerCount := runtime.NumCPU()
	uncompressedFiles := make(chan *webAssetFile, workerCount)
	compressedFiles := make(chan *webAssetFile, workerCount)
	bar := progressbar.Default(numFilesRecursive("web/dist"), "Compressing web assets")
	writeWorkers := &sync.WaitGroup{}
	for i := 0; i < workerCount; i++ {
		writeWorkers.Add(1)
		go func() {
			defer writeWorkers.Done()
			for {
				cf, ok := <-compressedFiles
				if !ok {
					return
				}
				f, err := os.OpenFile(cf.Path, os.O_WRONLY|os.O_TRUNC, 0644)
				if err != nil {
					panic(err)
				}
				if _, err := f.Write(cf.Data); err != nil {
					panic(err)
				}
				os.Rename(cf.Path, cf.Path+".br")
				info, err := os.Stat(cf.Path + ".br")
				if err != nil {
					panic(err)
				}
				compressedSize += info.Size()
				count++
			}
		}()
	}
	compressWorkers := &sync.WaitGroup{}
	for i := 0; i < workerCount; i++ {
		compressWorkers.Add(1)
		go func() {
			defer compressWorkers.Done()
			for {
				ucf, ok := <-uncompressedFiles
				if !ok {
					return
				}

				buf := new(bytes.Buffer)
				w := brotli.NewWriterLevel(buf, 10)
				w.Write(ucf.Data)
				w.Close()
				bar.Add(1)
				compressedFiles <- &webAssetFile{
					Path: ucf.Path,
					Data: buf.Bytes(),
				}
			}
		}()
	}
	if err := filepath.WalkDir("web/dist/_nuxt", func(path string, d fs.DirEntry, err error) error {
		// skip dirs
		if d.IsDir() {
			return nil
		}
		// compress files with brotli
		if strings.HasSuffix(path, ".br") {
			// already compressed
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		uncompressedSize += info.Size()
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		buf := bytes.NewBuffer(make([]byte, 0, info.Size()))
		_, err = io.Copy(buf, f)
		if err != nil {
			return err
		}
		f.Close()
		uncompressedFiles <- &webAssetFile{
			Path: path,
			Data: buf.Bytes(),
		}
		return nil
	}); err != nil {
		return err
	}
	close(uncompressedFiles)
	compressWorkers.Wait()
	close(compressedFiles)
	writeWorkers.Wait()
	bar.Close()

	fmt.Printf("Compressed %d files (%d MiB -> %d MiB)\n", count, uncompressedSize/1024/1024, compressedSize/1024/1024)
	return nil
}

func CleanDist() error {
	if _, err := os.Stat("web/dist/_nuxt"); err == nil {
		fmt.Println("Removing web/dist/_nuxt")
		if err := os.RemoveAll("web/dist/_nuxt"); err != nil {
			return err
		}
	}
	// remove all files in web/dist except .gitignore
	return filepath.Walk("web/dist", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if info.Name() == ".gitignore" {
			return nil
		}
		fmt.Println("Removing", path)
		return os.Remove(path)
	})
}

func numFilesRecursive(dir string) int64 {
	count := int64(0)
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".br") {
			return nil
		}
		count++
		return nil
	})
	return count
}
