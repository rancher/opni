package commands

import (
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
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

func importPreRunE(cmd *cobra.Command, args []string) error {
	if managementListenAddress == "" {
		panic("bug: managementListenAddress is empty")
	}

	client, err := remoteread.NewClient(cmd.Context(), remoteread.WithListenAddress(managementListenAddress))
	if err != nil {
		return err
	}

	remoteReadClient = client

	return nil
}
