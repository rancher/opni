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
}

func Push(ctx context.Context, client *dagger.Client, opts PushOpts) error {
	ctr := images.AlpineBase(client, images.WithPackages("helm")).
		Pipeline("Push OCI Charts").
		WithMountedDirectory("/src", opts.Dir).
		WithWorkdir("/src").
		WithSecretVariable("DOCKER_PASSWORD", opts.Target.Auth.Secret).
		WithExec([]string{
			"sh", "-c",
			fmt.Sprintf(`helm registry login --username="%s" --password="$DOCKER_PASSWORD" %s`, opts.Target.Auth.Username, opts.Target.Repo),
		}).
		WithExec([]string{"sh", "-c",
			fmt.Sprintf(`find . -type f -name "*.tgz" -exec helm push {} oci://%s/%s \;`, opts.Target.Repo, opts.Target.Auth.Username),
		})

	_, err := ctr.Sync(ctx)
	if err != nil {
		return fmt.Errorf("failed to push charts: %w", err)
	}
	return nil
}

type PublishOpts struct {
	Target         config.ChartTarget
	BuildContainer *dagger.Container
	Caches         config.Caches
}

func PublishToChartsRepo(ctx context.Context, client *dagger.Client, opts PublishOpts) error {
	workdir, err := opts.BuildContainer.Workdir(ctx)
	if err != nil {
		return fmt.Errorf("failed to get workdir: %w", err)
	}
	chartsMountPath := filepath.Join(workdir, "charts")
	assetsMountPath := filepath.Join(workdir, "assets")
	magefilesMountPath := filepath.Join(workdir, "magefiles")
	mageCacheDir, mageCache := opts.Caches.Mage()

	ctr := images.AlpineBase(client, images.WithPackages("github-cli")).
		Pipeline("Publish Charts").
		WithWorkdir(workdir).
		WithSecretVariable("GH_TOKEN", opts.Target.Auth.Secret).
		WithDirectory(workdir, client.Git(opts.Target.Repo, dagger.GitOpts{KeepGitDir: true}).Branch(opts.Target.Branch).Tree()).
		WithExec([]string{"gh", "auth", "setup-git"}).
		// WithExec([]string{"gh", "repo", "clone", opts.Target.Repo, ".", "--", "--branch", opts.Target.Branch, "--depth", "1", "--no-tags"}).
		WithDirectory(chartsMountPath, opts.BuildContainer.Directory(chartsMountPath)). // Important: WithDirectory merges the contents
		WithDirectory(assetsMountPath, opts.BuildContainer.Directory(assetsMountPath)).
		WithMountedDirectory(magefilesMountPath, opts.BuildContainer.Directory(magefilesMountPath)).
		WithMountedCache(mageCacheDir, mageCache).
		WithExec([]string{filepath.Join(mageCacheDir, "charts"), "charts:index"}).
		WithoutMount(magefilesMountPath).
		WithExec([]string{"git", "status", "--porcelain"}, dagger.ContainerWithExecOpts{RedirectStdout: "/git-status"})

	if data, err := ctr.File("/git-status").Contents(ctx); err != nil {
		return fmt.Errorf("git status failed: %w", err)
	} else if len(data) == 0 {
		fmt.Println("No changes to commit")
		return nil
	} else {
		fmt.Println("Will commit the following changes:\n" + string(data))
	}

	// if _, err := ctr.WithExec([]string{"git", "diff", "--quiet"}).Sync(ctx); err != nil {
	// 	var execErr *dagger.ExecError
	// 	if errors.As(err, &execErr) {
	// 		if execErr.ExitCode == 0 {
	// 			fmt.Println("No changes to commit")
	// 			return nil
	// 		} else {
	// 			fmt.Println("Changes detected")
	// 		}
	// 	} else {
	// 		return fmt.Errorf("git diff failed: %w", err)
	// 	}
	// }

	ctr = ctr.WithExec([]string{"git", "config", "user.name", opts.Target.Auth.Username}).
		WithExec([]string{"git", "config", "user.email", opts.Target.Auth.Email}).
		WithExec([]string{"git", "add", "charts/", "assets/", "index.yaml"}).
		WithExec([]string{"git", "commit", "-m", "Update charts"}).
		WithExec([]string{"git", "push", "origin", fmt.Sprintf("refs/heads/%[1]s:refs/heads/%[1]s", opts.Target.Branch)})

	_, err = ctr.Sync(ctx)
	if err != nil {
		return fmt.Errorf("failed to publish charts: %w", err)
	}
	return nil
}
