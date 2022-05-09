package commands

import "github.com/spf13/cobra"

func BuildBootstrapCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "bootstrap resource",
		Short: "Bootstraps new cluster capabilities",
		Long:  "See subcommands for more information.",
	}
	command.AddCommand(BuildBootstrapLoggingCmd())
	return command
}
