//go:build !noevents

package commands

import (
	"github.com/rancher/opni/pkg/events"
	"github.com/spf13/cobra"
)

var (
	shipperEndpoint string
)

func BuildEventsCmd() *cobra.Command {
	command := &cobra.Command{
		Use:   "events",
		Short: "Run the Kubernetes event collector",
		Long:  "The event collector ships Kubernetes events to an opni-shipper endpoint.",
		RunE:  doEvents,
	}

	command.Flags().StringVar(&shipperEndpoint, "endpoint", "http://opni-shipper:2021/log/ingest", "endpoint to post events to")
	return command
}

func doEvents(cmd *cobra.Command, _ []string) error {
	collector := events.NewEventCollector(cmd.Context(), shipperEndpoint)
	return collector.Run(cmd.Context().Done())
}

func init() {
	AddCommandsToGroup(OpniComponents, BuildEventsCmd())
}
