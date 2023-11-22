//go:build !minimal

package commands

import (
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/spf13/cobra"
)

func BuildMetricsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Interact with metrics plugin APIs",
	}

	cmd.AddCommand(BuildCortexAdminCmd())
	cmd.AddCommand(cortexops.BuildCortexOpsCmd())
	cmd.AddCommand(node.BuildNodeConfigurationCmd())

	ConfigureManagementCommand(cmd)
	ConfigureCortexAdminCommand(cmd)
	return cmd
}

func init() {
	AddCommandsToGroup(PluginAPIs, BuildMetricsCmd())
}
