package commands

import (
	"fmt"
	"time"

	"github.com/araddon/dateparse"
	"github.com/olebedev/when"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/plugins/cortex/pkg/apis/cortexadmin"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func BuildAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Cortex admin commands",
	}
	cmd.AddCommand(BuildQueryCmd())
	cmd.AddCommand(BuildQueryRangeCmd())
	ConfigureManagementCommand(cmd)
	return cmd
}

func BuildQueryCmd() *cobra.Command {
	var clusters []string
	cmd := &cobra.Command{
		Use:   "query <promql>",
		Args:  cobra.ExactArgs(1),
		Short: "Query time-series metrics from Cortex",
		Run: func(cmd *cobra.Command, args []string) {
			if len(clusters) == 0 {
				cl, err := client.ListClusters(cmd.Context(), &management.ListClustersRequest{})
				if err != nil {
					lg.Fatal(err)
				}
				for _, c := range cl.Items {
					clusters = append(clusters, c.Id)
				}
			}
			resp, err := adminClient.Query(cmd.Context(), &cortexadmin.QueryRequest{
				Tenants: clusters,
				Query:   args[0],
			})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(string(resp.GetData()))
		},
	}
	cmd.Flags().StringSliceVar(&clusters, "clusters", []string{}, "Cluster IDs to query (default=all)")
	return cmd
}

func BuildQueryRangeCmd() *cobra.Command {
	var clusters []string
	var start, end string
	var step time.Duration
	cmd := &cobra.Command{
		Use:   "query-range --start=<time> --end=<time> --step=<duration> <promql>",
		Args:  cobra.ExactArgs(1),
		Short: "Query time-series metrics from Cortex",
		Run: func(cmd *cobra.Command, args []string) {
			startTime := parseTimeOrDie(start)
			endTime := parseTimeOrDie(end)
			if len(clusters) == 0 {
				cl, err := client.ListClusters(cmd.Context(), &management.ListClustersRequest{})
				if err != nil {
					lg.Fatal(err)
				}
				for _, c := range cl.Items {
					clusters = append(clusters, c.Id)
				}
			}
			resp, err := adminClient.QueryRange(cmd.Context(), &cortexadmin.QueryRangeRequest{
				Tenants: clusters,
				Query:   args[0],
				Start:   timestamppb.New(startTime),
				End:     timestamppb.New(endTime),
				Step:    durationpb.New(step),
			})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(string(resp.GetData()))
		},
	}
	cmd.Flags().StringSliceVar(&clusters, "clusters", []string{}, "Cluster IDs to query (default=all)")
	cmd.Flags().StringVar(&start, "start", "30 minutes ago", "Start time")
	cmd.Flags().StringVar(&end, "end", "now", "End time")
	cmd.Flags().DurationVar(&step, "step", time.Minute, "Step size")
	return cmd
}

func parseTimeOrDie(timeStr string) time.Time {
	if t, err := dateparse.ParseAny(timeStr); err == nil {
		return t
	} else {
		t, err := when.EN.Parse(timeStr, time.Now())
		if err != nil || t == nil {
			lg.Fatal("could not interpret start time")
		}
		return t.Time
	}
}
