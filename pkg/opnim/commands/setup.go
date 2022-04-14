package commands

import (
	"strings"

	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/management"
	cliutil "github.com/rancher/opni/pkg/opnim/util"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"github.com/spf13/cobra"
)

var client management.ManagementClient
var adminClient cortexadmin.CortexAdminClient
var lg = logger.New()

func ConfigureManagementCommand(cmd *cobra.Command) {
	cmd.PersistentFlags().StringP("address", "a", "", "Management API address (default: auto-detect)")
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		address := cmd.Flag("address").Value.String()
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

		ac, err := cortexadmin.NewClient(cmd.Context(),
			cortexadmin.WithListenAddress(address))
		if err != nil {
			return err
		}
		adminClient = ac
		return nil
	}
}
