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
	"github.com/spf13/cobra"
)

var mgmtClient managementv1.ManagementClient
var managementListenAddress string
var lg = logger.New()

func ConfigureManagementCommand(cmd *cobra.Command) {
	if cmd.PersistentPreRunE == nil {
		cmd.PersistentPreRunE = managementPreRunE
	} else {
		cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			if err := managementPreRunE(cmd, args); err != nil {
				return err
			}
			return cmd.PersistentPreRunE(cmd, args)
		}
	}
}

func managementPreRunE(cmd *cobra.Command, _ []string) error {
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
	managementListenAddress = address

	c, err := clients.NewManagementClient(cmd.Context(),
		clients.WithAddress(address))
	if err != nil {
		return err
	}
	mgmtClient = c
	return nil
}
