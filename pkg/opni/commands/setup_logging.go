package commands

import (
	"github.com/rancher/opni/plugins/logging/pkg/apis/loggingadmin"
	"github.com/spf13/cobra"
)

var loggingAdminClient loggingadmin.LoggingAdminClient

func ConfigureLoggingAdminCommand(cmd *cobra.Command) {
	if cmd.PersistentPreRunE == nil {
		cmd.PersistentPreRunE = cortexAdminPreRunE
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

func loggingAdminPreRunE(cmd *cobra.Command, args []string) error {
	if managementListenAddress == "" {
		panic("bug: managementListenAddress is empty")
	}
	ac, err := loggingadmin.NewClient(cmd.Context(),
		loggingadmin.WithListenAddress(managementListenAddress))
	if err != nil {
		return err
	}
	loggingAdminClient = ac

	return nil
}
