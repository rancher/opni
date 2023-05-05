package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

func goBuild(args ...string) error {
	tag, _ := os.LookupEnv("BUILD_VERSION")
	tag = strings.TrimSpace(tag)

	version := "unversioned"
	if tag != "" {
		version = tag
	}

	defaultArgs := []string{
		"build",
		"-ldflags", fmt.Sprintf("-w -s -X github.com/rancher/opni/pkg/versions.Version=%s", version),
		"-trimpath",
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
