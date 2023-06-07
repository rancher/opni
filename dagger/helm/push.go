package helm

import (
	"context"
	"fmt"
	"path/filepath"

	"dagger.io/dagger"
	"github.com/rancher/opni/dagger/config"
	"github.com/rancher/opni/dagger/images"
)

type PushOpts struct {
	Target config.OCIChartTarget
	Dir    *dagger.Directory
	Charts []string
}

func Push(ctx context.Context, client *dagger.Client, opts PushOpts) error {
	ctr := images.AlpineBase(client, images.WithPackages("helm")).
		WithMountedDirectory("/src", opts.Dir).
		WithWorkdir("/src").
		WithSecretVariable("DOCKER_PASSWORD", opts.Target.Auth.Secret).
		WithExec([]string{
			"sh", "-c",
			fmt.Sprintf(`helm registry login --username="%s" --password="$DOCKER_PASSWORD" %s`, opts.Target.Auth.Username, opts.Target.Repo),
		})
	for _, chart := range opts.Charts {
		ctr = ctr.WithExec([]string{"helm", "push", chart, fmt.Sprintf("oci://%s/%s", opts.Target.Repo, opts.Target.Auth.Username)})
	}

	_, err := ctr.Sync(ctx)
	return err
}

type PublishOpts struct {
	Target         config.ChartTarget
	BuildContainer *dagger.Container
	Caches         config.Caches
}

func PublishToChartsRepo(ctx context.Context, client *dagger.Client, opts PublishOpts) error {
	workdir, err := opts.BuildContainer.Workdir(ctx)
	if err != nil {
		return err
	}
	chartsMountPath := filepath.Join(workdir, "charts")
	assetsMountPath := filepath.Join(workdir, "assets")
	magefilesMountPath := filepath.Join(workdir, "magefiles")
	mageCacheDir, mageCache := opts.Caches.Mage()
	ctr := images.AlpineBase(client, images.WithPackages("github-cli")).
		WithWorkdir(workdir).
		WithSecretVariable("GH_TOKEN", opts.Target.Auth.Secret).
		WithExec([]string{"gh", "auth", "setup-git"}).
		WithExec([]string{"gh", "repo", "clone", opts.Target.Repo, ".", "--", "--branch", opts.Target.Branch, "--depth", "1"}).
		WithDirectory(chartsMountPath, opts.BuildContainer.Directory(chartsMountPath)). // Important: WithDirectory merges the contents
		WithDirectory(assetsMountPath, opts.BuildContainer.Directory(assetsMountPath)).
		WithMountedDirectory(magefilesMountPath, opts.BuildContainer.Directory(magefilesMountPath)).
		WithMountedCache(mageCacheDir, mageCache).
		WithExec([]string{filepath.Join(mageCacheDir, "charts"), "charts:index"}).
		WithoutMount(mageCacheDir).
		WithExec([]string{"git", "diff", "--quiet"})
	if code, err := ctr.ExitCode(ctx); err != nil {
		return err
	} else if code != 0 {
		ctr = ctr.WithExec([]string{"git", "config", "user.name", opts.Target.Auth.Username}).
			WithExec([]string{"git", "config", "user.email", opts.Target.Auth.Email}).
			WithExec([]string{"git", "add", "charts/", "assets/", "index.yaml"}).
			WithExec([]string{"git", "commit", "-m", "Update charts"}).
			WithExec([]string{"git", "push", "origin", fmt.Sprintf("refs/heads/%[1]s:refs/heads/%[1]s", opts.Target.Branch)})
	} else {
		fmt.Println("No changes to commit")
	}
	_, err = ctr.Sync(ctx)
	return err
}
