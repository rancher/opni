package commands

import (
	"strings"

	cliutil "github.com/rancher/opni-monitoring/pkg/cli/util"
	"github.com/rancher/opni-monitoring/pkg/config"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/spf13/cobra"
)

var client management.ManagementClient
var lg = logger.New()

func BuildManageCmd() *cobra.Command {
	var address string
	manageCmd := &cobra.Command{
		Use:   "manage",
		Short: "Interact with the gateway's management API",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if address == "" {
				path, err := config.FindConfig()
				if err == nil {
					objects := cliutil.LoadConfigObjectsOrDie(path, lg)
					objects.Visit(func(obj *v1beta1.GatewayConfig) {
						address = strings.TrimPrefix(obj.Spec.Management.GRPCListenAddress, "tcp://")
					})
				}
			}
			if address == "" {
				address = management.DefaultManagementSocket()
			}
			c, err := management.NewClient(cmd.Context(),
				management.WithListenAddress(address))
			if err != nil {
				return err
			}
			client = c
			return nil
		},
	}
	manageCmd.PersistentFlags().StringVarP(&address, "address", "a", "", "Management API address (default: auto-detect)")
	manageCmd.AddCommand(BuildTokensCmd())
	manageCmd.AddCommand(BuildClustersCmd())
	manageCmd.AddCommand(BuildCertsCmd())
	manageCmd.AddCommand(BuildRolesCmd())
	manageCmd.AddCommand(BuildRoleBindingsCmd())
	manageCmd.AddCommand(BuildAccessMatrixCmd())
	manageCmd.AddCommand(BuildDebugCmd())
	return manageCmd
}
