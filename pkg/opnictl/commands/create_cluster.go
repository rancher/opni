package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
)

func BuildCreateClusterCmd() *cobra.Command {
	var createClusterCmd = &cobra.Command{
		Use:   "cluster",
		Short: "Create a new opni cluster",
		Long: fmt.Sprintf(`
		This command will install opni into the selected namespace using the Production API.
		For more information about the Production API, run %s.
		
		Your current kubeconfig context will be used to select the cluster to operate
		on, unless the --context flag is provided to select a specific context.`,
			chalk.Bold.TextStyle("opnictl help apis")),
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	return createClusterCmd
}
