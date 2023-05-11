//go:build !minimal

package commands

import (
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/spf13/cobra"
)

var adminClient cortexadmin.CortexAdminClient
var opsClient cortexops.CortexOpsClient
var nodeConfigClient node.NodeConfigurationClient

func ConfigureCortexAdminCommand(cmd *cobra.Command) {
	if cmd.PersistentPreRunE == nil {
		cmd.PersistentPreRunE = cortexAdminPreRunE
	} else {
		oldPreRunE := cmd.PersistentPreRunE
		cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
			if err := oldPreRunE(cmd, args); err != nil {
				return err
			}
			return cortexAdminPreRunE(cmd, args)
		}
	}
}

func cortexAdminPreRunE(cmd *cobra.Command, _ []string) error {
	adminClient = cortexadmin.NewCortexAdminClient(managementv1.UnderlyingConn(mgmtClient))
	opsClient = cortexops.NewCortexOpsClient(managementv1.UnderlyingConn(mgmtClient))
	nodeConfigClient = node.NewNodeConfigurationClient(managementv1.UnderlyingConn(mgmtClient))
	cmd.SetContext(cortexops.ContextWithCortexOpsClient(cmd.Context(), opsClient))
	return nil
}
