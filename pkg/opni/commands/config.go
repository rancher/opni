//go:build !minimal

package commands

import (
	configv1 "github.com/rancher/opni/pkg/config/v1"
	"github.com/spf13/cobra"
)

func BuildConfigCmd() *cobra.Command {
	cmd := configv1.BuildGatewayConfigCmd()
	ConfigureManagementCommand(cmd)
	ConfigureGatewayConfigCmd(cmd)
	return cmd
}

func init() {
	AddCommandsToGroup(ManagementAPI, BuildConfigCmd())
}
