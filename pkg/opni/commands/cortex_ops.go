package commands

import (
	"fmt"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	cliutil "github.com/rancher/opni/pkg/opni/util"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexops"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
)

func BuildCortexOpsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cortex-ops",
		Short: "Cortex cluster setup and configuration",
	}
	cmd.AddCommand(BuildClusterStatusCmd())
	cmd.AddCommand(BuildClusterConfigCmd())
	return cmd
}

func BuildClusterStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster-status",
		Short: "Show status of all cortex components",
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := managementv1.UnderlyingConn(mgmtClient)
			opsclient := cortexops.NewCortexOpsClient(cc)
			status, err := opsclient.GetClusterStatus(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			fmt.Println(cliutil.RenderCortexClusterStatus(status))
			return nil
		},
	}
	return cmd
}

func BuildClusterConfigCmd() *cobra.Command {
	var mode string
	cmd := &cobra.Command{
		Use:   "cluster-config",
		Short: "Show cortex configuration",
		Long: `
Modes:
(empty)    - show current configuration
"diff"     - show only values that differ from the defaults
"defaults" - show only the default values
`[1:],
		RunE: func(cmd *cobra.Command, args []string) error {
			cc := managementv1.UnderlyingConn(mgmtClient)
			opsclient := cortexops.NewCortexOpsClient(cc)
			resp, err := opsclient.GetClusterConfig(cmd.Context(), &cortexops.ClusterConfigRequest{
				ConfigModes: []string{mode},
			})
			if err != nil {
				return err
			}
			for _, config := range resp.ConfigYaml {
				fmt.Println(config)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "", "config mode")
	return cmd
}
