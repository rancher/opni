package cli

import (
	"github.com/kralicky/opni-monitoring/pkg/cli/commands"

	"github.com/spf13/cobra"
)

func BuildRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use: "opnim",
		Long: "                     _         \n" +
			"  ____  ____  ____  (_)\x1b[33m___ ___\x1b[0m\n" +
			" / __ \\/ __ \\/ __ \\/ \x1b[33m/ __ " + "`" + "__ \\\x1b[0m\n" +
			"/ /_/ / /_/ / / / / \x1b[33m/ / / / / /\x1b[0m\n" +
			"\\____/ .___/_/ /_/_\x1b[33m/_/ /_/ /_/\x1b[0m\n" +
			"    /_/                        \n" +
			"\x1b[33mMulti-(Cluster|Tenant) Monitoring\x1b[0m for Kubernetes\n",
	}
	rootCmd.AddCommand(commands.BuildGatewayCmd())
	rootCmd.AddCommand(commands.BuildAgentCmd())
	rootCmd.AddCommand(commands.BuildManageCmd())
	return rootCmd
}

func Execute() {
	BuildRootCmd().Execute()
}
