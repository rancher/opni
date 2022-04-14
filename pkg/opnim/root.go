package opnim

import (
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/opnim/commands"

	"github.com/spf13/cobra"
)

func BuildRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:  "opnim",
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
	return rootCmd
}

func Execute() {
	BuildRootCmd().Execute()
}
