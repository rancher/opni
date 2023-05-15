package config

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"dagger.io/dagger"
)

type BuilderConfig struct {
	Opni        ImageTarget
	OpniMinimal ImageTarget
	HelmOCI     ImageTarget
	Helm        ChartTarget

	Lint   bool
	Charts bool
	Test   bool
	Push   bool

	Export bool
}

type ImageTarget struct {
	Enabled   bool
	Repo      string
	Tag       string
	TagSuffix string
	Auth      AuthConfig
}

type ChartTarget struct {
	Enabled bool
	Repo    string
	Branch  string
	Auth    AuthConfig
}

type AuthConfig struct {
	Username string
	Email    string
	Secret   *dagger.Secret
}

func (it *ImageTarget) FlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.BoolVar(&it.Enabled, name, it.Enabled, "enable image build")
	fs.StringVar(&it.Repo, fmt.Sprintf("%s.repo", name), it.Repo, "image repo")
	fs.StringVar(&it.Tag, fmt.Sprintf("%s.tag", name), it.Tag, "image tag")
	fs.StringVar(&it.TagSuffix, fmt.Sprintf("%s.tag-suffix", name), it.TagSuffix, "image tag suffix")
	fs.StringVar(&it.Auth.Username, fmt.Sprintf("%s.username", name), it.Auth.Username, "image registry username")
	return fs
}

func (gt *ChartTarget) FlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.BoolVar(&gt.Enabled, name, gt.Enabled, "enable chart build")
	fs.StringVar(&gt.Repo, fmt.Sprintf("%s.repo", name), gt.Repo, "git repo")
	fs.StringVar(&gt.Branch, fmt.Sprintf("%s.branch", name), gt.Branch, "git branch")
	fs.StringVar(&gt.Auth.Username, fmt.Sprintf("%s.username", name), gt.Auth.Username, "git username")
	fs.StringVar(&gt.Auth.Email, fmt.Sprintf("%s.email", name), gt.Auth.Email, "git email")
	return fs
}

func (c *BuilderConfig) FlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.AddFlagSet(c.Opni.FlagSet("images.opni"))
	fs.AddFlagSet(c.OpniMinimal.FlagSet("images.minimal"))
	fs.AddFlagSet(c.HelmOCI.FlagSet("charts.oci"))
	fs.AddFlagSet(c.Helm.FlagSet("charts"))
	fs.BoolVar(&c.Lint, "lint", c.Lint, "run linters")
	fs.BoolVar(&c.Test, "test", c.Test, "run tests")
	fs.BoolVar(&c.Push, "push", c.Push, "push images to remote")
	fs.BoolVar(&c.Export, "export", c.Export, "export build artifacts to local filesystem")
	return fs
}

func (c *BuilderConfig) LoadSecretsFromEnv(client *dagger.Client) {
	if value, ok := os.LookupEnv("DOCKER_PASSWORD"); ok {
		dockerPassword := client.SetSecret("docker-password", value)
		c.Opni.Auth.Secret = dockerPassword
		c.OpniMinimal.Auth.Secret = dockerPassword
		c.HelmOCI.Auth.Secret = dockerPassword
	}
	if value, ok := os.LookupEnv("GH_TOKEN"); ok {
		ghToken := client.SetSecret("gh-token", value)
		c.Helm.Auth.Secret = ghToken
	}
}

type CacheVolume struct {
	*dagger.CacheVolume
	Path string
}

type Caches struct {
	GoMod       func() (string, *dagger.CacheVolume)
	GoBuild     func() (string, *dagger.CacheVolume)
	GoBin       func() (string, *dagger.CacheVolume)
	Mage        func() (string, *dagger.CacheVolume)
	Yarn        func() (string, *dagger.CacheVolume)
	NodeModules func() (string, *dagger.CacheVolume)
}
