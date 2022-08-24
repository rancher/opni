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
