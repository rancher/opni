package cli

import (
	"fmt"

	"github.com/urfave/cli"
)

var (
	Version    = "v0.0.0-dev"
	GitCommit  = "HEAD"
	DeleteAll  bool
	KubeConfig string
)

func NewApp() *cli.App {
	app := cli.NewApp()
	app.Name = "opnictl"
	app.Usage = "A ctl for the opni stack"
	app.Version = fmt.Sprintf("%s (%s)", Version, GitCommit)
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("%s version %s\n", app.Name, Version)
	}

	return app
}
