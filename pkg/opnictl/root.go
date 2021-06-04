package opnictl

import (
	"os"

	"github.com/rancher/opni/pkg/opnictl/commands"
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
	rootCmd.AddCommand(commands.InstallCmd)
	rootCmd.AddCommand(commands.UninstallCmd)
	rootCmd.AddCommand(commands.CreateCmd)
}
