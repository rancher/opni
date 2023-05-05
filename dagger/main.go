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
	"gopkg.in/yaml.v3"
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
				"internal/",
				"pkg/",
				"web/",
			},
		}),
		sourcePlugins: client.Host().Directory("plugins"),
		artifacts:     client.Directory(),
		hostArtifacts: getHostArtifactInfo(),
	}
	fmt.Printf("host artifacts: %+v\n", builder.hostArtifacts)
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
	mocksConfig          mocks
	pluginDirs           []string
	sourceWithoutPlugins *dagger.Directory
	sourcePlugins        *dagger.Directory

	artifactsMu   sync.Mutex
	exports       []func(ctx context.Context) error
	artifacts     *dagger.Directory
	hostArtifacts HostArtifactInfo
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
			if b.hostArtifacts.BuildIDs[name] == buildid {
				fmt.Printf("skipping %s\n", name)
				return nil
			} else {
				fmt.Printf("updating binary %s, buildid %s != %s\n", name, b.hostArtifacts.BuildIDs[name], buildid)
			}
		} else {
			fmt.Printf("error reading buildid for %s: %v\n", name, err)
		}
		_, err = file.Export(ctx, name)
		return err
	})
}

func (b *builder) run(ctx context.Context) error {
	// setup
	var mocksConfig mocks
	mockConfig, err := b.sourceWithoutPlugins.File("pkg/test/mock/mockgen.yaml").
		Contents(ctx)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal([]byte(mockConfig), &mocksConfig); err != nil {
		return err
	}
	b.mocksConfig = mocksConfig

	b.pluginDirs, err = b.sourcePlugins.Entries(ctx)
	if err != nil {
		return err
	}

	// run
	goCache := b.client.CacheVolume("go")
	goBuildCache := b.client.CacheVolume("gobuild")
	mageCache := b.client.CacheVolume("mage")
	// todo: cache bin/?

	mageBase := b.client.
		Container().
		From("golang:"+strings.TrimPrefix(runtime.Version(), "go")).
		WithMountedCache("/cache/go", goCache, dagger.ContainerWithMountedCacheOpts{Sharing: dagger.Shared}).
		WithMountedCache("/cache/go-build", goBuildCache).
		WithMountedCache("/cache/mage", mageCache).
		WithEnvVariable("GOMODCACHE", "/cache/go").
		WithEnvVariable("GOCACHE", "/cache/go-build").
		WithEnvVariable("MAGEFILE_CACHE", "/cache/mage").
		WithExec([]string{"apt", "update"}).
		WithExec([]string{"go", "install", "github.com/magefile/mage@latest"}).
		WithWorkdir(b.workdir)

	gen := b.generate(mageBase.Pipeline("generate"))

	buildCtr := mageBase.
		WithMountedDirectory(b.workdir, gen.Directory(b.workdir)).
		WithMountedDirectory(b.workdir, b.sourceWithoutPlugins).
		WithMountedDirectory(b.workdir+"/plugins", b.sourcePlugins)

	b.build(buildCtr)

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
