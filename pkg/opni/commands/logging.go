//go:build !minimal

package commands

import (
	"github.com/spf13/cobra"
)

var forceLoggingAdminV1 bool

func BuildLoggingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logging",
		Short: "Interact with logging plugin APIs",
	}

	cmd.AddCommand(BuildLoggingUpgradeCmd())
	cmd.AddCommand(BuildLoggingBackendCmd())
	cmd.Flags().BoolVar(&forceLoggingAdminV1, "v1", false, "Force command to use the v1 api")

	ConfigureManagementCommand(cmd)
	ConfigureLoggingAdminCommand(cmd)

	return cmd
}

func init() {
	AddCommandsToGroup(PluginAPIs, BuildLoggingCmd())
}
