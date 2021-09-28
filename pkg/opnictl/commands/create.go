// Package commands contains the opnictl sub-commands.
package commands

import (
	"github.com/spf13/cobra"
)

func BuildCreateCmd() *cobra.Command {
	var createCmd = &cobra.Command{
		Use:   "create resource",
		Short: "Create new Opni resources",
		Long:  "See subcommands for more information.",
	}
	createCmd.AddCommand(BuildCreateDemoCmd())
	createCmd.AddCommand(BuildCreateClusterCmd())
	return createCmd
}
