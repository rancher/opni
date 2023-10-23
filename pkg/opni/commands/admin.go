//go:build !minimal

package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"slices"

	"github.com/araddon/dateparse"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/samber/lo"
	"gopkg.in/yaml.v3"

	"github.com/olebedev/when"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/metrics/compat"
	"github.com/rancher/opni/pkg/opni/cliutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func BuildCortexAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Cortex admin tools",
	}
	cmd.AddCommand(BuildQueryCmd())
	cmd.AddCommand(BuildQueryRangeCmd())
	cmd.AddCommand(BuildStorageInfoCmd())
	cmd.AddCommand(BuildFlushBlocksCmd())
	cmd.AddCommand(BuildCortexStatusCmd())
	cmd.AddCommand(BuildCortexConfigCmd())
	cmd.AddCommand(BuildClusterStatsCmd())
	cmd.AddCommand(BuildCortexAdminRulesCmd())

	return cmd
}

func BuildCortexAdminRulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rules",
		Short: "Cortex admin rules",
	}
	cmd.AddCommand(BuildListRulesCmd())
	cmd.AddCommand(BuildDeleteRuleGroupsCmd())
	cmd.AddCommand(BuildLoadRuleGroupsCmd())
	return cmd
}

func BuildDeleteRuleGroupsCmd() *cobra.Command {
	var clusters string
	var namespace string

	cmd := &cobra.Command{
		Use:   "delete <groupname>",
		Short: "Delete prometheus rule groups of the given name from Cortex",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			_, err := adminClient.DeleteRule(cmd.Context(), &cortexadmin.DeleteRuleRequest{
				ClusterId: clusters,
				Namespace: namespace,
				GroupName: args[0],
			})
			if err != nil {
				lg.Error("error", logger.Err(err))
				return
			}
			fmt.Println("Rule Group deleted successfully")
		},
	}
	cmd.Flags().StringVarP(&clusters, "cluster", "c", "", "The clusters to delete the rule from")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "The namespace to delete the rule from")
	return cmd
}

func BuildLoadRuleGroupsCmd() *cobra.Command {
	var clusters []string
	var namespace string
	cmd := &cobra.Command{
		Use:   "load <rulegroupfile>",
		Short: "Creates/Updates prometheus rule groups into Cortex from a valid prometheus rule group file",
		Long:  "See https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/#recording-rules for more information about the expected input format",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			//read file and validate contents

			clMeta, err := mgmtClient.ListClusters(cmd.Context(), &managementv1.ListClustersRequest{})
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			}
			cl := lo.Map(clMeta.Items, func(cl *corev1.Cluster, _ int) string {
				return cl.Id
			})
			if len(clusters) == 0 {
				clusters = cl
			} else {
				// Important to validate here !! Since cortex has no knowledge of available opni clusters,
				// it will accept any valid yaml content and therefore could silently fail/ destroy itself
				// by writing unsuported names to its (remote) store
				for _, c := range clusters {
					if !slices.Contains(cl, c) {
						lg.Error(fmt.Sprintf("invalid cluster id %s", c))
						os.Exit(1)
					}
				}
			}
			yamlContent, err := os.ReadFile(args[0])
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			}
			rgs, errors := rulefmt.Parse(yamlContent)
			if len(errors) > 0 {
				for _, err := range errors {
					lg.Error("error", logger.Err(err))
				}
				lg.Error("Failed to parse rule group file")
				os.Exit(1)
			}

			var wg sync.WaitGroup
			for _, cl := range clusters {
				cl := cl
				wg.Add(1)
				go func() {
					defer wg.Done()
					for _, group := range rgs.Groups {
						_, err := adminClient.LoadRules(cmd.Context(), &cortexadmin.LoadRuleRequest{
							Namespace:   namespace,
							ClusterId:   cl,
							YamlContent: util.Must(yaml.Marshal(group)),
						})
						if err != nil {
							lg.Error(fmt.Sprintf("Failed to load rule group :\n `%s`\n\n for cluster `%s`", string(util.Must(yaml.Marshal(group))), cl))
						} else {
							fmt.Printf("Successfully loaded rule group `%s` for clusterId `%s`\n", group.Name, cl)
						}
					}
				}()
			}
			wg.Wait()
		},
	}
	cmd.Flags().StringSliceVar(&clusters, "cluster", []string{}, "The clusters to apply the rule to (default=all)")
	cmd.Flags().StringVar(&namespace, "namespace", "", "namespace is a cortex identifier to help organize rules (default=\"default\")")
	return cmd
}

func BuildListRulesCmd() *cobra.Command {
	var clusters []string
	var ruleFilter []string
	var healthFilter []string
	var stateFilter []string
	var ruleNameFilter string
	var groupNameFilter string
	var namespaceFilter string
	var outputFormat string
	var invalidDiagnosticRequested bool
	var all bool
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "List recording and/or alerting rules from Cortex",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		Run: func(cmd *cobra.Command, args []string) {
			if outputFormat != "table" && outputFormat != "json" {
				lg.Error("invalid output format")
				os.Exit(1)
			}

			// since cortexadmin Server no longer embeds an opni management client,
			// when we request to list invalid rules, we must pass in all clusters so we can have
			// enough information to weed out completely invalid rules
			if len(clusters) == 0 || invalidDiagnosticRequested || all {
				cl, err := mgmtClient.ListClusters(cmd.Context(), &managementv1.ListClustersRequest{})
				if err != nil {
					lg.Error("fatal", logger.Err(err))
					os.Exit(1)
				}
				clusters = lo.Map(cl.Items, func(cl *corev1.Cluster, _ int) string {
					return cl.Id
				})
			}

			resp, err := adminClient.ListRules(cmd.Context(), &cortexadmin.ListRulesRequest{
				ClusterId:       clusters,
				RuleType:        ruleFilter,
				HealthFilter:    healthFilter,
				StateFilter:     stateFilter,
				RuleNameRegexp:  ruleNameFilter,
				GroupNameRegexp: groupNameFilter,
				NamespaceRegexp: namespaceFilter,
				ListInvalid:     &invalidDiagnosticRequested,
				RequestAll:      &all,
			})
			if err != nil {
				lg.Error("error", logger.Err(err))
				return
			}
			if outputFormat == "table" {
				fmt.Println(cliutil.RenderCortexRules(resp))
			} else {
				fmt.Println(string(util.Must(json.Marshal(resp))))
			}
		},
	}
	cmd.Flags().StringSliceVar(&clusters, "clusters", []string{}, "Cluster IDs to query (default=all)")
	cmd.Flags().StringSliceVar(&ruleFilter, "rule", []string{}, "Rule type to list (default=all)")
	cmd.Flags().StringSliceVar(&healthFilter, "health", []string{}, "Rule health status to list (default=all)")
	cmd.Flags().StringSliceVar(&stateFilter, "state", []string{}, "Rule state to list (default=all)")
	cmd.Flags().StringVar(&groupNameFilter, "group-name", "", "Group names to list (supports go regex) (default=all)")
	cmd.Flags().StringVar(&ruleNameFilter, "rule-name", "", "Rule names to list (supports go regex) (default=all)")
	cmd.Flags().StringVar(&namespaceFilter, "namespace", "", "Namespaces to match against (supports go regex) (default=all)")
	cmd.Flags().BoolVar(&invalidDiagnosticRequested, "invalid", false, "List invalid rules (default=false)")
	cmd.Flags().BoolVar(&all, "all", false, "List all rules present in cortex, regardless of cluster (default=false)")
	cmd.Flags().StringVar(&outputFormat, "output", "table", "Output format : table,json (default=table)")
	return cmd
}

func BuildQueryCmd() *cobra.Command {
	var clusters []string
	cmd := &cobra.Command{
		Use:               "query <promql>",
		Short:             "Query time-series metrics from Cortex",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		Run: func(cmd *cobra.Command, args []string) {
			if len(clusters) == 0 {
				cl, err := mgmtClient.ListClusters(cmd.Context(), &managementv1.ListClustersRequest{})
				if err != nil {
					lg.Error("fatal", logger.Err(err))
					os.Exit(1)
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
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
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
		Use:               "query-range --start=<time> --end=<time> --step=<duration> <promql>",
		Short:             "Query time-series metrics from Cortex",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		Run: func(cmd *cobra.Command, args []string) {
			startTime := parseTimeOrDie(start)
			endTime := parseTimeOrDie(end)
			if len(clusters) == 0 {
				cl, err := mgmtClient.ListClusters(cmd.Context(), &managementv1.ListClustersRequest{})
				if err != nil {
					lg.Error("fatal", logger.Err(err))
					os.Exit(1)
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
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
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
	}
	t, err := when.EN.Parse(timeStr, time.Now())
	if err != nil || t == nil {
		lg.Error("could not interpret start time")
		os.Exit(1)
	}
	return t.Time
}

func BuildStorageInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "storage-info [<cluster-id> ...]",
		Short: "Show cluster storage metrics",
		Args:  cobra.MinimumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeClusters(cmd, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				cl, err := mgmtClient.ListClusters(cmd.Context(), &managementv1.ListClustersRequest{})
				if err != nil {
					lg.Error("fatal", logger.Err(err))
					os.Exit(1)
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
			queryResp, err := compat.UnmarshalPrometheusResponse(resp.GetData())
			if err != nil {
				return err
			}
			var samples []*model.Sample
			if queryResp.V.Type() == model.ValVector {
				samples = append(samples, queryResp.V.(model.Vector)...)
			}

			fmt.Println(cliutil.RenderMetricSamples(samples))
			return nil
		},
	}
	return cmd
}

func BuildFlushBlocksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "flush-blocks",
		Short: "Flush in-memory ingester TSDB data to long-term storage",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := adminClient.FlushBlocks(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			lg.Info("Success")
			return nil
		},
	}
	return cmd
}

func BuildCortexStatusCmd() *cobra.Command {
	var outputFormat string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show status of all cortex components",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := adminClient.GetCortexStatus(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				fmt.Println(err)
				// continue, status may contain partial information
			}
			switch outputFormat {
			case "json":
				fmt.Println(protojson.Format(status))
			case "table":
				fmt.Println(cliutil.RenderCortexClusterStatus(status))
			default:
				return fmt.Errorf("unknown output format: %s", outputFormat)
			}
			return err
		},
	}
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table|json)")
	return cmd
}

func BuildCortexConfigCmd() *cobra.Command {
	var mode string
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show cortex configuration",
		Long: `
Modes:
(empty)    - show current configuration
"diff"     - show only values that differ from the defaults
"defaults" - show only the default values
`[1:],
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := adminClient.GetCortexConfig(cmd.Context(), &cortexadmin.ConfigRequest{
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

func BuildClusterStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-clusters",
		Short: "List clusters",
		Run: func(cmd *cobra.Command, args []string) {
			t, err := mgmtClient.ListClusters(cmd.Context(), &managementv1.ListClustersRequest{})
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			}
			var clusterStats *cortexadmin.UserIDStatsList
			var healthStatus []*corev1.HealthStatus
			for _, c := range t.Items {
				stat, err := mgmtClient.GetClusterHealthStatus(cmd.Context(), c.Reference())
				if err != nil {
					healthStatus = append(healthStatus, &corev1.HealthStatus{})
				} else {
					healthStatus = append(healthStatus, stat)
				}
			}

			stats, err := adminClient.AllUserStats(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				lg.With(
					logger.Err(err),
				).Warn("failed to query cortex stats")
			}
			clusterStats = stats
			fmt.Println(cliutil.RenderClusterListWithStats(t, healthStatus, clusterStats))
		},
	}
	return cmd
}
