package opni

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/opni/commands"
	"github.com/rancher/opni/pkg/opni/common"
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
	ctx, ca := context.WithCancel(context.Background())
	c := make(chan os.Signal, 2)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	rootCmd := BuildRootCmd()
	go func() {
		sig := <-c
		switch sig {
		case syscall.SIGINT:
			rootCmd.PrintErrln("\nShutting down... (press Ctrl+C again to force)")
		default:
			rootCmd.PrintErrf("Received %s, shutting down...", sig.String())
		}
		ca()
		<-c
		os.Exit(1)
	}()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		rootCmd.PrintErrln(err.Error())
		os.Exit(1)
	}
}

func patchUsageTemplate(cmd *cobra.Command) {
	defaultUsageTemplate := cmd.UsageTemplate()
	w, _, _ := term.GetSize(int(os.Stdout.Fd()))
	cmd.SetUsageTemplate(strings.ReplaceAll(defaultUsageTemplate, ".FlagUsages", fmt.Sprintf(".FlagUsagesWrapped %d", w)))
}
