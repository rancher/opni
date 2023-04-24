//go:build !noplugins

package commands

import (
	"github.com/spf13/cobra"
)

func BuildMetricsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Interact with metrics plugin APIs",
	}

	cmd.AddCommand(BuildCortexAdminCmd())
	cmd.AddCommand(BuildCortexOpsCmd())
	cmd.AddCommand(BuildMetricsConfigCmd())

	ConfigureManagementCommand(cmd)
	ConfigureCortexAdminCommand(cmd)
	return cmd
}

func init() {
	AddCommandsToGroup(PluginAPIs, BuildMetricsCmd())
}
