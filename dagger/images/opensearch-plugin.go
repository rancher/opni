package images

import (
	"context"

	"dagger.io/dagger"
	"github.com/rancher/opni/dagger/config"
)

func BuildOpensearchDashboardsPlugin(ctx context.Context, client *dagger.Client, caches config.Caches) error {
	osdSrc := client.Git("https://github.com/opensearch-project/OpenSearch-Dashboards.git", dagger.GitOpts{KeepGitDir: true}).
		Branch("2.4").
		Tree()

	opniPluginSrc := client.Git("https://github.com/rancher/opni-ui", dagger.GitOpts{KeepGitDir: true}).
		Branch("plugin").
		Tree()

	_, err := NodeBase(client).Pipeline("Opensearch Dashboards Plugin").
		WithMountedCache(caches.Yarn()).
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
