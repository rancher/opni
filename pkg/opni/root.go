package opni

import (
	"os"

	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/opni/commands"

	"github.com/spf13/cobra"
)

func BuildRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:  "opni",
		Long: logger.AsciiLogo(),
	}
	rootCmd.AddCommand(commands.BuildAccessMatrixCmd())
	rootCmd.AddCommand(commands.BuildAdminCmd())
	rootCmd.AddCommand(commands.BuildAgentCmd())
	rootCmd.AddCommand(commands.BuildBootstrapCmd())
	rootCmd.AddCommand(commands.BuildCertsCmd())
	rootCmd.AddCommand(commands.BuildClustersCmd())
	rootCmd.AddCommand(commands.BuildDebugCmd())
	rootCmd.AddCommand(commands.BuildGatewayCmd())
	rootCmd.AddCommand(commands.BuildRoleBindingsCmd())
	rootCmd.AddCommand(commands.BuildRolesCmd())
	rootCmd.AddCommand(commands.BuildTokensCmd())
	rootCmd.AddCommand(commands.BuildVersionCmd())
	rootCmd.AddCommand(commands.BuildManagerCmd())
	rootCmd.AddCommand(commands.BuildCortexCmd())
	rootCmd.AddCommand(commands.BuildRealtimeCmd())
	rootCmd.AddCommand(commands.BuildEventsCmd())
	rootCmd.AddCommand(commands.BuildHooksCmd())
	return rootCmd
}

func Execute() {
	if err := BuildRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
