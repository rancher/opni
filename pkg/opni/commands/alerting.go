//go:build !minimal

package commands

import (
	"flag"

	"github.com/rancher/opni/internal/alerting/amtool"
	"github.com/spf13/cobra"
)

func BuildAlertingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alerting",
		Short: "Interact with alerting plugin APIs",
	}

	cmd.AddCommand(BuildAmtoolCmd())

	ConfigureManagementCommand(cmd)
	return cmd
}

func BuildAmtoolCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "amtool",
		Short:              "vendored prometheus/alertmanager amtool",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			flag.CommandLine = flag.NewFlagSet("amtool", flag.ExitOnError)
			amtool.Execute(args)
		},
	}
	return cmd
}
func init() {
	AddCommandsToGroup(PluginAPIs, BuildAlertingCmd())
}
