package commands

import (
	"fmt"
	"time"

	"github.com/araddon/dateparse"
	"github.com/olebedev/when"
	"github.com/prometheus/common/model"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/metrics/unmarshal"
	cliutil "github.com/rancher/opni/pkg/opni/util"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
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
	cmd.AddCommand(BuildStorageInfoCmd())
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
				cl, err := mgmtClient.ListClusters(cmd.Context(), &managementv1.ListClustersRequest{})
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
				cl, err := mgmtClient.ListClusters(cmd.Context(), &managementv1.ListClustersRequest{})
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

func BuildStorageInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "storage-info [<cluster-id> ...]",
		Short: "Show cluster storage metrics",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				cl, err := mgmtClient.ListClusters(cmd.Context(), &managementv1.ListClustersRequest{})
				if err != nil {
					lg.Fatal(err)
				}
				for _, c := range cl.Items {
					args = append(args, c.Id)
				}
			}
			resp, err := adminClient.Query(cmd.Context(), &cortexadmin.QueryRequest{
				Tenants: args,
				Query:   "cortex_bucket_blocks_count",
			})
			if err != nil {
				return err
			}
			queryResp, err := unmarshal.UnmarshalPrometheusResponse(resp.GetData())
			if err != nil {
				return err
			}
			var samples []*model.Sample
			switch queryResp.V.Type() {
			case model.ValVector:
				samples = append(samples, queryResp.V.(model.Vector)...)
			}
			fmt.Println(cliutil.RenderMetricSamples(samples))
			return nil
		},
	}
	return cmd
}
