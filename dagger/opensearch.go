package main

import (
	"context"
	"fmt"

	"dagger.io/dagger"
	"github.com/rancher/opni/dagger/images"
)

func (b *Builder) BuildOpensearchDashboardsPlugin(ctx context.Context) error {
	osdSrc := b.client.Git("https://github.com/opensearch-project/OpenSearch-Dashboards.git", dagger.GitOpts{KeepGitDir: true}).
		Branch("2.4").
		Tree()

	opniPluginSrc := b.client.Git("https://github.com/rancher/opni-ui", dagger.GitOpts{KeepGitDir: true}).
		Branch("plugin").
		Tree()

	_, err := images.NodeBase(b.client).Pipeline("Opensearch Dashboards Plugin").
		WithMountedCache(b.caches.Yarn()).
		WithEnvVariable("YARN_CACHE_FOLDER", "/cache/yarn").
		WithDirectory("/src", osdSrc).
		WithDirectory("/src/plugins/opni-dashboards-plugin", opniPluginSrc).
		WithWorkdir("/src/plugins/opni-dashboards-plugin").
		WithExec([]string{"sed", "-i", `s/{VERSION}/test/g`, "package.json", "opensearch_dashboards.json"}).
		WithWorkdir("/src").
		WithExec([]string{"yarn", "osd", "bootstrap"}).
		WithWorkdir("/src/plugins/opni-dashboards-plugin").
		WithExec([]string{"yarn", "install"}).
		WithExec([]string{"yarn", "build"}).
		Directory("/src/plugins/opni-dashboards-plugin/build").
		Export(ctx, "dist/dashboard-plugin/")
	return err
}

func (b *Builder) BuildOpensearchDashboardsImage() *dagger.Container {
	ctr := b.client.Container().
		Pipeline("Opensearch Dashboards Image").
		From(fmt.Sprintf("opensearchproject/opensearch-dashboards:%s", b.Images.Opensearch.Build.DashboardsVersion)).
		WithExec([]string{"opensearch-dashboards-plugin", "install",
			fmt.Sprintf("https://github.com/rancher/opni-ui/releases/download/plugin-%[1]s/opni-dashboards-plugin-%[1]s.zip", b.Images.Opensearch.Build.PluginVersion),
		})
	return ctr
}

func (b *Builder) BuildOpensearchImage() *dagger.Container {
	entrypointScript := b.sources.File("images/opensearch/entrypoint.sh")
	ctr := b.client.Container().
		Pipeline("Opensearch Image").
		From(fmt.Sprintf("opensearchproject/opensearch:%s", b.Images.Opensearch.Build.DashboardsVersion)).
		WithExec([]string{"opensearch-plugin", "-s", "install", "-b",
			fmt.Sprintf("https://github.com/rancher/opni-ingest-plugin/releases/download/v%s/opnipreprocessing.zip", b.Images.Opensearch.Build.PluginVersion),
		}).
		WithFile("/usr/share/opensearch/opensearch-docker-entrypoint.sh", entrypointScript)
	return ctr
}

func (b *Builder) BuildOpensearchUpdateServiceImage() *dagger.Container {
	ctr := b.client.Container().
		Pipeline("Opensearch Update Service Image").
		From("rancher/opni-python-base:3.8").
		WithDirectory(".", b.sources.Directory("aiops/")).
		WithExec([]string{"pip", "install", "-r", "requirements.txt"}).
		WithEntrypoint([]string{"python", "opni-opensearch-update-service/opensearch-update-service/app/main.py"})
	return ctr
}
