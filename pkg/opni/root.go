package opni

import (
	"context"
	"os"

	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/opni/commands"
	"github.com/rancher/opni/pkg/opni/common"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/ttacon/chalk"
	"k8s.io/kubectl/pkg/util/templates"

	"github.com/spf13/cobra"
)

func BuildRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "opni",
		Long:         chalk.ResetColor.Color(logger.AsciiLogo()),
		SilenceUsage: true,
	}
	rootCmd.PersistentFlags().BoolVar(&common.DisableUsage, "disable-usage", false, "Disable anonymous Opni usage tracking.")

	groups := templates.CommandGroups{
		{
			Message: "Opni Components:",
			Commands: []*cobra.Command{
				commands.BuildAgentCmd(),
				commands.BuildAgentV2Cmd(),
				commands.BuildCortexCmd(),
				commands.BuildEventsCmd(),
				commands.BuildGatewayCmd(),
				commands.BuildManagerCmd(),
				commands.BuildRealtimeCmd(),
				commands.BuildClientCmd(),
			},
		},
		{
			Message: "Management API:",
			Commands: []*cobra.Command{
				commands.BuildCapabilityCmd(),
				commands.BuildCertsCmd(),
				commands.BuildClustersCmd(),
				commands.BuildKeyringsCmd(),
				commands.BuildRolesCmd(),
				commands.BuildRoleBindingsCmd(),
				commands.BuildTokensCmd(),
			},
		},
		{
			Message: "Plugin APIs:",
			Commands: []*cobra.Command{
				commands.BuildMetricsCmd(),
			},
		},
		{
			Message: "Utilities:",
			Commands: []*cobra.Command{
				commands.BuildAccessMatrixCmd(),
				commands.BuildBootstrapCmd(),
				commands.BuildHooksCmd(),
			},
		},
		{
			Message: "Debug:",
			Commands: []*cobra.Command{
				commands.BuildDebugCmd(),
			},
		},
	}

	groups.Add(rootCmd)
	rootCmd.AddCommand(commands.CompletionCmd)
	rootCmd.AddCommand(commands.BuildVersionCmd())
	fe := templates.ActsAsRootCommand(rootCmd, nil, groups...)
	fe.ExposeFlags(rootCmd, "disable-usage")

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
