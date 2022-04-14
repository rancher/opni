package commands

import (
	"fmt"

	cliutil "github.com/rancher/opni/pkg/opnim/util"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
)

func BuildCertsCmd() *cobra.Command {
	certsCmd := &cobra.Command{
		Use:   "certs",
		Short: "Manage certificates",
	}
	certsCmd.AddCommand(BuildCertsInfoCmd())
	ConfigureManagementCommand(certsCmd)
	return certsCmd
}

func BuildCertsInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show certificate information",
		Run: func(cmd *cobra.Command, args []string) {
			t, err := client.CertsInfo(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderCertInfoChain(t.Chain))
		},
	}
}
