//go:build !minimal

package commands

import (
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/plugins/logging/apis/loggingadmin"
	"github.com/spf13/cobra"
)

var (
	loggingAdminV2Client loggingadmin.LoggingAdminV2Client
)

func ConfigureLoggingAdminCommand(cmd *cobra.Command) {
	if cmd.PersistentPreRunE == nil {
		cmd.PersistentPreRunE = loggingAdminPreRunE
	} else {
		oldPreRunE := cmd.PersistentPreRunE
		cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			if err := oldPreRunE(cmd, args); err != nil {
				return err
			}
			return loggingAdminPreRunE(cmd, args)
		}
	}
}

func loggingAdminPreRunE(_ *cobra.Command, _ []string) error {
	loggingAdminV2Client = loggingadmin.NewLoggingAdminV2Client(managementv1.UnderlyingConn(mgmtClient))
	return nil
}
