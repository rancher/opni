//go:build !minimal

package commands

import (
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	configv1 "github.com/rancher/opni/pkg/config/v1"
	"github.com/spf13/cobra"
)

var configClient configv1.GatewayConfigClient

func ConfigureGatewayConfigCmd(cmd *cobra.Command) {
	if cmd.PersistentPreRunE == nil {
		cmd.PersistentPreRunE = gatewayConfigPreRunE
	} else {
		oldPreRunE := cmd.PersistentPreRunE
		cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			if err := oldPreRunE(cmd, args); err != nil {
				return err
			}
			return gatewayConfigPreRunE(cmd, args)
		}
	}
}

func gatewayConfigPreRunE(cmd *cobra.Command, _ []string) error {
	configClient = configv1.NewGatewayConfigClient(managementv1.UnderlyingConn(mgmtClient))
	cmd.SetContext(configv1.GatewayConfigContextInjector.ContextWithClient(cmd.Context(), configClient))
	return nil
}
