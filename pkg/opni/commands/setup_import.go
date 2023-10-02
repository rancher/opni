//go:build !minimal

package commands

import (
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
	"github.com/spf13/cobra"
)

var remoteReadClient remoteread.RemoteReadGatewayClient

func ConfigureImportCommand(cmd *cobra.Command) {
	if cmd.PersistentPreRunE == nil {
		cmd.PersistentPreRunE = importPreRunE
	} else {
		oldPreRunE := cmd.PersistentPreRunE
		cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			if err := oldPreRunE(cmd, args); err != nil {
				return err
			}

			return importPreRunE(cmd, args)
		}
	}
}

func importPreRunE(_ *cobra.Command, _ []string) error {
	remoteReadClient = remoteread.NewRemoteReadGatewayClient(managementv1.UnderlyingConn(mgmtClient))
	return nil
}
