//go:build !minimal

package commands

import (
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/spf13/cobra"
)

func BuildAuthCmd() *cobra.Command {
	authCmd := managementv1.BuildLocalPasswordCmd()
	ConfigureManagementCommand(authCmd)
	ConfigureAuthCommand(authCmd)
	return authCmd
}

func ConfigureAuthCommand(cmd *cobra.Command) {
	if cmd.PersistentPreRunE == nil {
		cmd.PersistentPreRunE = cortexAdminPreRunE
	} else {
		oldPreRunE := cmd.PersistentPreRunE
		cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			if err := oldPreRunE(cmd, args); err != nil {
				return err
			}
			return authPreRunE(cmd, args)
		}
	}
}

func authPreRunE(cmd *cobra.Command, _ []string) error {
	authClient := managementv1.NewLocalPasswordClient(managementv1.UnderlyingConn(mgmtClient))
	cmd.SetContext(managementv1.LocalPasswordContextInjector.ContextWithClient(cmd.Context(), authClient))
	return nil
}

func init() {
	AddCommandsToGroup(ManagementAPI, BuildAuthCmd())
}
