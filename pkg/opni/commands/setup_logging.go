//go:build !noplugins

package commands

import (
	"github.com/rancher/opni/plugins/logging/pkg/apis/loggingadmin"
	"github.com/spf13/cobra"
)

var (
	loggingAdminClient   loggingadmin.LoggingAdminClient
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

func loggingAdminPreRunE(cmd *cobra.Command, _ []string) error {
	if managementListenAddress == "" {
		panic("bug: managementListenAddress is empty")
	}
	ac, err := loggingadmin.NewClient(cmd.Context(),
		loggingadmin.WithListenAddress(managementListenAddress))
	if err != nil {
		return err
	}
	loggingAdminClient = ac

	ac2, err := loggingadmin.NewV2Client(cmd.Context(),
		loggingadmin.WithListenAddress(managementListenAddress))
	if err != nil {
		return err
	}
	loggingAdminV2Client = ac2

	return nil
}
