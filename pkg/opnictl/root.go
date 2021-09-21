// Package opnictl contains the root of the Opnictl command tree.
package opnictl

import (
	"context"
	"os"
	"time"

	"github.com/rancher/opni/pkg/opnictl/common"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/rancher/opni/pkg/opnictl/commands"
	"github.com/rancher/opni/pkg/opnictl/helptopics"
	"github.com/spf13/cobra"
)

func BuildRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use: "opnictl",
		Long: `                     _ 
  ____  ____  ____  (_)
 / __ \/ __ \/ __ \/ / 
/ /_/ / /_/ / / / / /  
\____/ .___/_/ /_/_/   
    /_/                
 AIOps for Kubernetes
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	// Flags
	rootCmd.PersistentFlags().StringVarP(&common.NamespaceFlagValue, "namespace", "n", "opni",
		"namespace to install resources to")
	rootCmd.PersistentFlags().StringVar(&common.ExplicitPathFlagValue, clientcmd.RecommendedConfigPathFlag, "",
		"explicit path to kubeconfig file")
	rootCmd.PersistentFlags().StringVar(&common.ContextOverrideFlagValue, "context", "",
		"Kubernetes context (defaults to current-context)")
	rootCmd.PersistentFlags().DurationVar(&common.TimeoutFlagValue, "timeout", 5*time.Minute,
		"Duration to wait for Create/Delete operations before timing out")

	// Sub-commands
	rootCmd.AddCommand(commands.BuildInstallCmd())
	rootCmd.AddCommand(commands.BuildUninstallCmd())
	rootCmd.AddCommand(commands.BuildCreateCmd())
	rootCmd.AddCommand(commands.BuildDeleteCmd())
	rootCmd.AddCommand(commands.BuildGetCmd())
	rootCmd.AddCommand(commands.CompletionCmd)

	// Help topics
	rootCmd.AddCommand(helptopics.ApisHelpCmd)

	return rootCmd
}

func Execute() {
	common.LoadDefaultClientConfig()
	if err := BuildRootCmd().ExecuteContext(context.Background()); err != nil {
		os.Exit(1)
	}
}
