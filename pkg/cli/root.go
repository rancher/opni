package cli

import (
	"github.com/rancher/opni-monitoring/pkg/cli/commands"
	"github.com/rancher/opni-monitoring/pkg/logger"

	"github.com/spf13/cobra"
)

func BuildRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:  "opnim",
		Long: logger.AsciiLogo(),
	}
	rootCmd.AddCommand(commands.BuildBootstrapCmd())
	rootCmd.AddCommand(commands.BuildGatewayCmd())
	rootCmd.AddCommand(commands.BuildAgentCmd())
	rootCmd.AddCommand(commands.BuildManageCmd())
	return rootCmd
}

func Execute() {
	BuildRootCmd().Execute()
}
