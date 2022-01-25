package commands

import (
	"github.com/kralicky/opni-monitoring/pkg/logger"
	"github.com/kralicky/opni-monitoring/pkg/management"
	"github.com/spf13/cobra"
)

var client management.ManagementClient
var lg = logger.New().Named("management")

func BuildManageCmd() *cobra.Command {
	var address string
	manageCmd := &cobra.Command{
		Use:   "manage",
		Short: "Interact with the gateway's management API",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			c, err := management.NewClient(cmd.Context(),
				management.WithListenAddress(address))
			if err != nil {
				return err
			}
			client = c
			return nil
		},
	}
	manageCmd.PersistentFlags().StringVarP(&address, "address", "a",
		management.DefaultManagementSocket(), "Management API address")
	manageCmd.AddCommand(BuildTokensCmd())
	manageCmd.AddCommand(BuildClustersCmd())
	manageCmd.AddCommand(BuildCertsCmd())
	manageCmd.AddCommand(BuildRolesCmd())
	manageCmd.AddCommand(BuildRoleBindingsCmd())
	manageCmd.AddCommand(BuildAccessMatrixCmd())
	return manageCmd
}
