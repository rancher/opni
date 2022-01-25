package commands

import (
	"fmt"

	cliutil "github.com/kralicky/opni-monitoring/pkg/cli/util"
	"github.com/spf13/cobra"
)

func BuildCertsCmd() *cobra.Command {
	certsCmd := &cobra.Command{
		Use:   "certs",
		Short: "Manage certificates",
	}
	certsCmd.AddCommand(BuildCertsInfoCmd())
	return certsCmd
}

func BuildCertsInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show certificate information",
		Run: func(cmd *cobra.Command, args []string) {
			t, err := client.CertsInfo(cmd.Context())
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderCertInfoChain(t.Chain))
		},
	}
}
