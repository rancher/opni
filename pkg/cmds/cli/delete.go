package cli

import (
	"github.com/urfave/cli"
)

func NewDeleteCommand(action func(*cli.Context) error) cli.Command {
	return cli.Command{
		Name:      "delete",
		Usage:     "delete opni stack",
		UsageText: "opnictl install [OPTIONS]",
		Action:    action,
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:        "kubeconfig",
				EnvVar:      "KUBECONFIG",
				Destination: &KubeConfig,
				Usage:       "Kubeconfig file to access the kubernetes cluster",
				Value:       "~/.kube/config",
			},
			cli.BoolFlag{
				Name:        "all",
				Destination: &DeleteAll,
			},
		},
	}
}
