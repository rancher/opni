package commands

import (
	cliutil "github.com/rancher/opni/pkg/util/opnictl"
	"github.com/spf13/cobra"
)

var DeleteCmd = &cobra.Command{
	Use:   "delete resource",
	Short: "Delete existing opni resources",
}

var scheme = cliutil.CreateScheme()

var DeleteDemoCmd = &cobra.Command{
	Use:   "demo-cluster",
	Short: "Delete an existing opni demo cluster",
	Run: func(cmd *cobra.Command, args []string) {

	},
}

func init() {
	DeleteCmd.AddCommand(DeleteDemoCmd)
}
