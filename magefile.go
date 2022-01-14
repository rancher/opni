//go:build mage

package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"emperror.dev/errors"

	"github.com/kralicky/ragu/pkg/ragu"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

var Default = All

func All() {
	mg.SerialDeps(Build)
}

func Build() error {
	mg.Deps(Generate)
	return sh.RunWith(map[string]string{
		"CGO_ENABLED": "0",
	}, mg.GoCmd(), "build", "-ldflags", "-w -s", "-o", "bin/opnim", "./cmd/opnim")
}

func Test() error {
	return sh.RunV(mg.GoCmd(), "run", "github.com/onsi/ginkgo/v2/ginkgo",
		"-r",
		"--randomize-suites",
		"--fail-on-pending",
		"--keep-going",
		"--cover",
		"--coverprofile=cover.out",
		"--race",
		"--trace",
		"--timeout=10m")
}

func Docker() error {
	mg.Deps(Build)
	return sh.RunWithV(map[string]string{
		"DOCKER_BUILDKIT": "1",
	}, "docker", "build", "-t", "kralicky/opni-monitoring", ".")
}

type mockgenConfig struct {
	Source string
	Dest   string
	Types  []string
}

func Generate() error {
	wg := sync.WaitGroup{}

	var mu sync.Mutex
	var generateErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		protos, err := ragu.GenerateCode("pkg/management/management.proto", true)
		if err != nil {
			mu.Lock()
			generateErr = errors.Append(generateErr, err)
			mu.Unlock()
			return
		}
		for _, f := range protos {
			path := filepath.Join("pkg/management", f.GetName())
			if info, err := os.Stat(path); err == nil {
				if info.Mode()&0200 == 0 {
					if err := os.Chmod(path, 0644); err != nil {
						mu.Lock()
						generateErr = errors.Append(generateErr, err)
						mu.Unlock()
						return
					}
				}
			}
			if err := os.WriteFile(path, []byte(f.GetContent()), 0444); err != nil {
				mu.Lock()
				generateErr = errors.Append(generateErr, err)
				mu.Unlock()
				return
			}
			if err := os.Chmod(path, 0444); err != nil {
				mu.Lock()
				generateErr = errors.Append(generateErr, err)
				mu.Unlock()
				return
			}
		}
	}()

	for _, cfg := range []mockgenConfig{
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
	} {
		wg.Add(1)
		go func(cfg mockgenConfig) {
			defer wg.Done()
			err := sh.RunV(mg.GoCmd(), "run", "github.com/golang/mock/mockgen",
				"-source="+cfg.Source,
				"-destination="+cfg.Dest,
				strings.Join(cfg.Types, ","))
			if err != nil {
				mu.Lock()
				generateErr = errors.Append(generateErr, err)
				mu.Unlock()
			}
		}(cfg)
	}

	wg.Wait()
	return generateErr
}
