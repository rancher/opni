package delete

import (
	"context"

	cmds "github.com/rancher/opnictl/pkg/cmds/cli"
	"github.com/rancher/opnictl/pkg/deploy"
	"github.com/urfave/cli"
)

func Run(c *cli.Context) error {
	ctx := context.Background()

	sc, err := deploy.NewContext(ctx, cmds.KubeConfig)
	if err != nil {
		return err
	}

	if err := sc.Start(ctx); err != nil {
		return err
	}

	if err := deploy.Delete(ctx, sc, cmds.DeleteAll); err != nil {
		return err
	}
	return nil
}
