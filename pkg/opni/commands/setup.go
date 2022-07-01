package commands

import (
	"strings"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/config"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	cliutil "github.com/rancher/opni/pkg/opni/util"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"github.com/spf13/cobra"
)

var mgmtClient managementv1.ManagementClient
var adminClient cortexadmin.CortexAdminClient
var lg = logger.New()

func ConfigureManagementCommand(cmd *cobra.Command) {
	cmd.PersistentFlags().StringP("address", "a", "", "Management API address (default: auto-detect)")
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		tracing.Configure("cli")
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
			address = managementv1.DefaultManagementSocket()
		}
		c, err := clients.NewManagementClient(cmd.Context(),
			clients.WithAddress(address))
		if err != nil {
			return err
		}
		mgmtClient = c

		ac, err := cortexadmin.NewClient(cmd.Context(),
			cortexadmin.WithListenAddress(address))
		if err != nil {
			return err
		}
		adminClient = ac
		return nil
	}
}
