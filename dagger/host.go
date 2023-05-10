package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"
)

type HostInfo struct {
	PluginDirs    []fs.DirEntry
	MockgenConfig struct {
		Mocks []struct {
			Source string `yaml:"source"`
			Dest   string `yaml:"dest"`
		} `yaml:"mocks"`
	}
	BuildIDs map[string]string // filename:buildid, filenames start with 'bin/'
}

func getHostInfo() (info HostInfo) {
	info.BuildIDs = map[string]string{}

	rootDir, ok := getRootDir()
	if !ok {
		return
	}

	// find all the binaries in the root dir
	binDir := filepath.Join(rootDir, "bin")
	if _, err := os.Stat(binDir); err == nil {
		filepath.WalkDir(binDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			// trim the root path prefix
			relPath, err := filepath.Rel(rootDir, path)
			if err != nil {
				return err
			}
			info.BuildIDs[relPath] = getBuildID(path)
			return nil
		})
	}

	mockConfig, err := os.ReadFile(filepath.Join(rootDir, "pkg/test/mock/mockgen.yaml"))
	if err != nil {
		panic(err)
	}
	if err := yaml.Unmarshal([]byte(mockConfig), &info.MockgenConfig); err != nil {
		panic(err)
	}
	info.PluginDirs, _ = os.ReadDir(filepath.Join(rootDir, "plugins"))

	return
}

func getRootDir() (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	// find the root go.mod with "module github.com/rancher/opni"
	for cwd != "/" {
		if data, err := os.ReadFile(filepath.Join(cwd, "go.mod")); err == nil {
			if modfile.ModulePath(data) == "github.com/rancher/opni" {
				// found it
				return cwd, true
			}
		}
		cwd = filepath.Dir(cwd)
	}

	return "", false
}

func getBuildID(path string) string {
	cmd := exec.Command("go", "tool", "buildid", path)
	out, err := cmd.Output()
	if err != nil {
		panic(err)
	}
	return strings.TrimSpace(string(out))
}
