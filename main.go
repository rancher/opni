//go:generate go run pkg/codegen/cleanup/main.go
//go:generate /bin/rm -rf pkg/generated
//go:generate go run pkg/codegen/main.go

package main

import (
	"os"

	cmds "github.com/rancher/opnictl/pkg/cmds/cli"
	"github.com/rancher/opnictl/pkg/cmds/delete"
	"github.com/rancher/opnictl/pkg/cmds/install"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func main() {
	app := cmds.NewApp()
	app.Commands = []cli.Command{
		cmds.NewInstallCommand(install.Run),
		cmds.NewDeleteCommand(delete.Run),
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}
