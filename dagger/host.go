package main

import (
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

type HostArtifactInfo struct {
	BuildIDs map[string]string // filename:buildid, filenames start with './bin/'
}

func getHostArtifactInfo() (info HostArtifactInfo) {
	rootDir, ok := getRootDir()
	if !ok {
		return
	}

	// find all the binaries in the root dir
	binDir := filepath.Join(rootDir, "bin")
	if _, err := os.Stat(binDir); err != nil {
		return
	}

	info.BuildIDs = map[string]string{}

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
	return string(out)
}
