package commands

import "github.com/spf13/cobra"

func BuildBootstrapCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "bootstrap resource",
		Short: "Bootstraps new Opni resources",
		Long:  "See subcommands for more information.",
	}
	command.AddCommand(BuildBootstrapLoggingCommand())
	return command
}
