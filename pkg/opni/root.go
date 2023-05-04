package opni

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/opni/commands"
	"github.com/rancher/opni/pkg/opni/common"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/ttacon/chalk"
	"golang.org/x/term"

	"github.com/spf13/cobra"
)

func BuildRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "opni",
		Long:         chalk.ResetColor.Color(logger.AsciiLogo()),
		SilenceUsage: true,
	}

	patchUsageTemplate(rootCmd)

	rootCmd.AddGroup(commands.AllGroups...)
	rootCmd.AddCommand(commands.AllCommands...)

	rootCmd.InitDefaultCompletionCmd()
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

func patchUsageTemplate(cmd *cobra.Command) {
	defaultUsageTemplate := cmd.UsageTemplate()
	w, _, _ := term.GetSize(int(os.Stdout.Fd()))
	cmd.SetUsageTemplate(strings.ReplaceAll(defaultUsageTemplate, ".FlagUsages", fmt.Sprintf(".FlagUsagesWrapped %d", w)))
}
