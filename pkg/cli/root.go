package cli

import (
	"log"

	"github.com/kralicky/opni-gateway/pkg/cli/commands"

	"github.com/spf13/cobra"
)

func BuildRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use: "opni-gateway",
	}
	rootCmd.AddCommand(commands.BuildServeCmd())
	rootCmd.AddCommand(commands.BuildToolCmd())
	return rootCmd
}

func Execute() {
	if err := BuildRootCmd().Execute(); err != nil {
		log.Fatal(err)
	}
}
