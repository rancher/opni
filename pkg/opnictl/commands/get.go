package commands

import "github.com/spf13/cobra"

var GetCmd = &cobra.Command{
	Use:   "get resource",
	Short: "Show existing opni resources",
}

var GetDemoCmd = &cobra.Command{
	Use:   "demo-cluster name",
	Args:  cobra.NoArgs,
	Short: "Show existing OpniDemo resources",
	Run: func(cmd *cobra.Command, args []string) {

	},
}
