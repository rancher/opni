package opni

import (
	"context"
	"os"

	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/opni/commands"
	"github.com/rancher/opni/pkg/opni/common"
	"github.com/rancher/opni/pkg/util/waitctx"

	"github.com/spf13/cobra"
)

func BuildRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "opni",
		Long:         logger.AsciiLogo(),
		SilenceUsage: true,
	}
	rootCmd.AddCommand(commands.BuildAccessMatrixCmd())
	rootCmd.AddCommand(commands.BuildAdminCmd())
	rootCmd.AddCommand(commands.BuildAgentCmd())
	rootCmd.AddCommand(commands.BuildAgentV2Cmd())
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
	rootCmd.AddCommand(commands.BuildCapabilityCmd())
	rootCmd.AddCommand(commands.BuildKeyringsCmd())

	rootCmd.PersistentFlags().BoolVar(&common.DisableUsage, "disable-usage", false, "Disable anonymous Opni usage tracking.")

	return rootCmd
}

func Execute() {
	ctx, ca := context.WithCancel(waitctx.Background())
	if err := BuildRootCmd().ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
	ca()
	waitctx.Wait(ctx)
}
