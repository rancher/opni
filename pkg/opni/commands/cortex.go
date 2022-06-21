package commands

import (
	"flag"

	cortex_internal "github.com/rancher/opni/internal/cortex"
	"github.com/rancher/opni/pkg/tracing"
	"github.com/spf13/cobra"
)

func BuildCortexCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "cortex",
		Short:              "Embedded cortex",
		DisableFlagParsing: true,
		Run: func(cmd *cobra.Command, args []string) {
			tracing.Configure("cortex")
			flag.CommandLine = flag.NewFlagSet("cortex", flag.ExitOnError)
			cortex_internal.Main(append([]string{"cortex"}, args...))
		},
	}

	return cmd
}
