//go:build !noalertmanager

package commands

import (
	"flag"

	alertmanager_internal "github.com/rancher/opni/internal/alertmanager"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/spf13/cobra"
)

func BuildAlertingCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "alertmanager",
		Short:              "Run the embedded Alertmanager server",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			tracing.Configure("alertmanager")
			flag.CommandLine = flag.NewFlagSet("alertmanager", flag.ExitOnError)
			alertmanager_internal.Main(append([]string{"alertmanager"}, args...))
		},
	}
	return cmd
}

func init() {
	AddCommandsToGroup(OpniComponents, BuildAlertingCmd())
}
