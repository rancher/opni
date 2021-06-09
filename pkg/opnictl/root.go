// Package opnictl contains the root of the Opnictl command tree.
package opnictl

import (
	"os"
	"time"

	. "github.com/rancher/opni/pkg/opnictl/common"

	"github.com/rancher/opni/pkg/opnictl/commands"
	"github.com/rancher/opni/pkg/opnictl/helptopics"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
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

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Flags
	rootCmd.PersistentFlags().StringVarP(&NamespaceFlagValue, "namespace", "n", "opni-demo",
		"namespace to install resources to")
	rootCmd.PersistentFlags().StringVar(&NamespaceFlagValue, "context", "",
		"Kubernetes context (defaults to current-context)")
	rootCmd.PersistentFlags().DurationVar(&TimeoutFlagValue, "timeout", 2*time.Minute,
		"Duration to wait for Create/Delete operations before timing out")

	// Sub-commands
	rootCmd.AddCommand(commands.InstallCmd)
	rootCmd.AddCommand(commands.UninstallCmd)
	rootCmd.AddCommand(commands.CreateCmd)
	rootCmd.AddCommand(commands.DeleteCmd)
	rootCmd.AddCommand(commands.GetCmd)
	rootCmd.AddCommand(commands.CompletionCmd)

	// Help topics
	rootCmd.AddCommand(helptopics.ApisHelpCmd)
}
