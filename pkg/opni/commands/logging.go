package commands

import (
	"github.com/spf13/cobra"
)

func BuildLoggingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logging",
		Short: "Interact with logging plugin APIs",
	}

	cmd.AddCommand(BuildLoggingUpgradeCmd())

	ConfigureManagementCommand(cmd)
	ConfigureLoggingAdminCommand(cmd)

	return cmd
}

func init() {
	AddCommandsToGroup(PluginAPIs, BuildLoggingCmd())
}
