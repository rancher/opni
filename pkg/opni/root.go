package opni

import (
	"context"
	"os"

	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/opni/commands"
	"github.com/rancher/opni/pkg/opni/common"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/ttacon/chalk"

	"github.com/spf13/cobra"
)

func BuildRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "opni",
		Long:         chalk.ResetColor.Color(logger.AsciiLogo()),
		SilenceUsage: true,
	}

	rootCmd.AddGroup(commands.AllGroups...)
	rootCmd.AddCommand(commands.AllCommands...)

	rootCmd.AddCommand(commands.CompletionCmd)
	rootCmd.AddCommand(commands.BuildVersionCmd())

	rootCmd.PersistentFlags().BoolVar(&common.DisableUsage, "disable-usage", false, "Disable anonymous Opni usage tracking.")
	rootCmd.PersistentFlags().StringP("address", "a", "", "Management API address (default: auto-detect)")

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
