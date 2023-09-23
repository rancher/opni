//go:build example_cli

package commands

import (
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/plugins/example/pkg/example"
	"github.com/spf13/cobra"
)

func BuildExampleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "example",
		Short: "Interact with example plugin APIs",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			client := example.NewConfigClient(managementv1.UnderlyingConn(mgmtClient))
			cmd.SetContext(example.ContextWithConfigClient(cmd.Context(), client))
			return nil
		},
	}

	cmd.AddCommand(example.BuildConfigCmd())

	ConfigureManagementCommand(cmd)
	return cmd
}

func init() {
	AddCommandsToGroup(PluginAPIs, BuildExampleCmd())
}
