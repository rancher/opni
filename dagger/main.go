package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"dagger.io/dagger"
	"golang.org/x/sync/errgroup"
)

func main() {
	ctx := context.Background()
	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stdout))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	builder := &builder{
		ctx:     ctx,
		client:  client,
		workdir: "/src",
		sourceWithoutPlugins: client.Host().Directory(".", dagger.HostDirectoryOpts{
			Include: []string{
				"go.mod",
				"go.sum",
				"apis/",
				"cmd/",
				"controllers/",
				"internal/alerting/",
				"internal/cli/",
				"internal/cortex/",
				"pkg/",
				"web/*.go",
			},
			Exclude: []string{
				"pkg/test/",
			},
		}).WithNewFile("web/dist/.gitkeep", ""),
		sourcePlugins: client.Host().Directory("plugins", dagger.HostDirectoryOpts{
			Exclude: []string{
				"*/test/",
			},
		}),
		sourceWeb: client.Host().Directory(".", dagger.HostDirectoryOpts{
			Include: []string{
				"web/",
			},
			Exclude: []string{
				"web/dist/",
				"web/node_modules/",
			},
		}),
		artifacts: client.Directory(),
		hostInfo:  getHostInfo(),
	}
	fmt.Printf("host artifacts: %+v\n", builder.hostInfo)
	defer client.Close()
	if err := builder.run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type builder struct {
	ctx                  context.Context
	client               *dagger.Client
	workdir              string
	sourceWithoutPlugins *dagger.Directory
	sourcePlugins        *dagger.Directory
	sourceWeb            *dagger.Directory

	artifactsMu sync.Mutex
	exports     []func(ctx context.Context) error
	artifacts   *dagger.Directory
	hostInfo    HostInfo
}

func (b *builder) ExportToRoot(dir *dagger.Directory, opts ...dagger.DirectoryWithDirectoryOpts) {
	b.artifactsMu.Lock()
	defer b.artifactsMu.Unlock()
	b.exports = append(b.exports, func(ctx context.Context) error {
		_, err := dir.Export(ctx, ".")
		return err
	})
}

func (b *builder) ExportToPath(ctr *dagger.Container, name string, opts ...dagger.DirectoryWithFileOpts) {
	file := ctr.File(name)
	b.artifactsMu.Lock()
	defer b.artifactsMu.Unlock()
	b.exports = append(b.exports, func(ctx context.Context) error {
		name = filepath.Clean(name)
		buildid, err := ctr.File(name + ".buildid").Contents(ctx)
		if err == nil {
			buildid = strings.TrimSpace(string(buildid))
			if b.hostInfo.BuildIDs[name] == buildid {
				fmt.Printf("skipping %s\n", name)
				return nil
			} else {
				fmt.Printf("updating binary %s, buildid %s != %s\n", name, b.hostInfo.BuildIDs[name], buildid)
			}
		} else {
			fmt.Printf("error reading buildid for %s: %v\n", name, err)
		}
		_, err = file.Export(ctx, name)
		return err
	})
}

func (b *builder) run(ctx context.Context) error {
	// run
	goCache := b.client.CacheVolume("go")
	goBuildCache := b.client.CacheVolume("gobuild")
	mageCache := b.client.CacheVolume("mage")
	nodeModulesCache := b.client.CacheVolume("node_modules")
	// todo: cache bin/?

	nodeBase := b.client.
		Container().
		From("node:14").
		WithMountedCache("/cache/node_modules", nodeModulesCache).
		WithEnvVariable("NODE_PATH", "/cache/node_modules").
		WithWorkdir(filepath.Join(b.workdir, "web"))

	mageBase := b.client.
		Container().
		From("golang:"+strings.TrimPrefix(runtime.Version(), "go")).
		WithMountedCache("/cache/go", goCache).
		WithMountedCache("/cache/go-build", goBuildCache).
		WithMountedCache("/cache/mage", mageCache).
		WithEnvVariable("GOMODCACHE", "/cache/go").
		WithEnvVariable("GOCACHE", "/cache/go-build").
		WithEnvVariable("MAGEFILE_CACHE", "/cache/mage").
		WithExec([]string{"go", "install", "github.com/magefile/mage@latest"}).
		WithWorkdir(b.workdir)

	gen := b.generate(mageBase.Pipeline("generate"))

	mageBuild := mageBase.
		WithMountedDirectory(b.workdir, gen.Directory(b.workdir)).
		WithMountedDirectory(b.workdir, b.sourceWithoutPlugins).
		WithMountedDirectory(b.workdir+"/plugins", b.sourcePlugins)

	nodeBuild := nodeBase.
		WithMountedDirectory(b.workdir, gen.Directory(b.workdir)).
		WithMountedDirectory(b.workdir, b.sourceWeb)

	b.build(mageBuild, nodeBuild)

	eg, ctx := errgroup.WithContext(ctx)
	for _, export := range b.exports {
		export := export
		eg.Go(func() error {
			return export(ctx)
		})
	}
	return eg.Wait()
}

func getChecksum(ctx context.Context, f *dagger.File) ([]byte, error) {
	contents, err := f.Contents(ctx)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(contents))
	return sum[:], nil
}
