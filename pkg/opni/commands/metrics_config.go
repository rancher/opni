//go:build !minimal

package commands

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/opni/cliutil"
	"github.com/rancher/opni/pkg/render"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"
)

func BuildMetricsConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "View and configure the metrics capability",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			defaultConfig, err := nodeConfigClient.GetDefaultNodeConfiguration(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			allAgents, err := mgmtClient.ListClusters(cmd.Context(), &managementv1.ListClustersRequest{})
			if err != nil {
				return err
			}

			items := make([]render.MetricsNodeConfigInfo, len(allAgents.Items))
			for i, agent := range allAgents.Items {
				var trailer metadata.MD
				spec, err := nodeConfigClient.GetNodeConfiguration(cmd.Context(), agent.Reference(), grpc.Trailer(&trailer))
				if err != nil {
					return err
				}
				items[i] = render.MetricsNodeConfigInfo{
					Id:            agent.Id,
					HasCapability: capabilities.Has(agent, capabilities.Cluster(wellknown.CapabilityMetrics)),
					Spec:          spec,
					IsDefault:     node.IsDefaultConfig(trailer),
				}
			}

			fmt.Println(render.RenderMetricsNodeConfigs(items, defaultConfig))
			return nil
		},
	}
	cmd.AddCommand(BuildMetricsConfigSetCmd())
	cmd.AddCommand(BuildMetricsConfigGetCmd())
	cmd.AddCommand(BuildMetricsConfigResetCmd())
	cmd.AddCommand(BuildMetricsConfigSetDefaultCmd())
	cmd.AddCommand(BuildMetricsConfigGetDefaultCmd())
	cmd.AddCommand(BuildMetricsConfigResetDefaultCmd())
	return cmd
}

func BuildMetricsConfigSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "set <cluster-id> [config]",
		Aliases: []string{"edit"},
		Short:   "Set metrics configuration for a cluster",
		Args:    cobra.RangeArgs(1, 2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return completeClusters(cmd, args, toComplete)
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterID := args[0]
			var spec *node.MetricsCapabilitySpec
			var err error
			if len(args) == 2 {
				spec, err = parseSpec(args[1])
				if err != nil {
					return err
				}
			} else {
				// fetch the current spec, open it in an editor (json), and then
				// parse the result
				conf, err := nodeConfigClient.GetNodeConfiguration(cmd.Context(), &corev1.Reference{
					Id: clusterID,
				})
				if err != nil {
					return err
				}
				updated, err := cliutil.EditInteractive(conf)
				if err != nil {
					return err
				}
				if cmp.Equal(conf, updated, protocmp.Transform()) {
					fmt.Println("No changes made")
					return nil
				}
				spec = updated
			}
			_, err = nodeConfigClient.SetNodeConfiguration(cmd.Context(), &node.NodeConfigRequest{
				Node: &corev1.Reference{
					Id: clusterID,
				},
				Spec: spec,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Metrics configuration set for cluster %q\n", clusterID)
			return nil
		},
	}
	return cmd
}

func BuildMetricsConfigGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <cluster-id> [cluster-id ...]",
		Short: "Get metrics configuration for one or more clusters",
		Args:  cobra.MinimumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeClusters(cmd, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			infos := make([]render.MetricsNodeConfigInfo, 0, len(args))
			for _, clusterID := range args {
				ref := &corev1.Reference{Id: clusterID}
				var trailer metadata.MD
				spec, err := nodeConfigClient.GetNodeConfiguration(cmd.Context(), ref, grpc.Trailer(&trailer))
				if err != nil {
					return err
				}
				cluster, err := mgmtClient.GetCluster(cmd.Context(), ref)
				if err != nil {
					return err
				}

				infos = append(infos, render.MetricsNodeConfigInfo{
					Id:            clusterID,
					HasCapability: capabilities.Has(cluster, capabilities.Cluster(wellknown.CapabilityMetrics)),
					Spec:          spec,
					IsDefault:     node.IsDefaultConfig(trailer),
				})
			}
			fmt.Println(render.RenderMetricsNodeConfigs(infos, nil))
			return nil
		},
	}
	return cmd
}

func BuildMetricsConfigResetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset <cluster-id> [cluster-id ...]",
		Args:  cobra.MinimumNArgs(1),
		Short: "Reset the metrics capability configuration for a cluster to the default",
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeClusters(cmd, args, toComplete, func(c *corev1.Cluster) bool {
				// filter for clusters that are using the default config
				var trailer metadata.MD
				client := node.NewNodeConfigurationClient(managementv1.UnderlyingConn(mgmtClient))
				_, err := client.GetNodeConfiguration(cmd.Context(), c.Reference(), grpc.Trailer(&trailer))
				return err == nil && node.IsDefaultConfig(trailer)
			})
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, clusterID := range args {
				// set the config with a nil spec to reset it
				_, err := nodeConfigClient.SetNodeConfiguration(cmd.Context(), &node.NodeConfigRequest{
					Node: &corev1.Reference{
						Id: clusterID,
					},
					Spec: nil,
				})
				if err != nil {
					fmt.Printf("Failed to reset metrics capability configuration for cluster %q: %v\n", clusterID, err)
				}
			}
			return nil
		},
	}
	return cmd
}

func BuildMetricsConfigSetDefaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "set-default [config]",
		Aliases: []string{"edit-default"},
		Short:   "Set the default metrics configuration (applies to all clusters without a specific configuration)",
		Args:    cobra.RangeArgs(0, 1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var spec *node.MetricsCapabilitySpec
			var err error
			if len(args) == 1 {
				spec, err = parseSpec(args[0])
				if err != nil {
					return err
				}
			} else {
				// fetch the current spec, open it in an editor (json), and then
				// parse the result
				spec, err = nodeConfigClient.GetDefaultNodeConfiguration(cmd.Context(), &emptypb.Empty{})
				if err != nil {
					return err
				}
				updated, err := cliutil.EditInteractive(spec)
				if err != nil {
					return err
				}
				if cmp.Equal(spec, updated, protocmp.Transform()) {
					fmt.Println("No changes made")
					return nil
				}
				spec = updated
			}

			_, err = nodeConfigClient.SetDefaultNodeConfiguration(cmd.Context(), spec)
			if err != nil {
				return err
			}
			fmt.Println("Default metrics configuration set")
			return nil
		},
	}
	return cmd
}

func BuildMetricsConfigGetDefaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-default",
		Short: "Get the default metrics configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := nodeConfigClient.GetDefaultNodeConfiguration(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}

			fmt.Println(render.RenderDefaultNodeConfig(spec))
			return nil
		},
	}
	return cmd
}

func BuildMetricsConfigResetDefaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset-default",
		Short: "Reset the default metrics configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := nodeConfigClient.SetDefaultNodeConfiguration(cmd.Context(), &node.MetricsCapabilitySpec{})
			if err != nil {
				return err
			}
			fmt.Println("Default metrics configuration reset")
			return nil
		},
	}
	return cmd
}

func parseSpec(input string) (*node.MetricsCapabilitySpec, error) {
	spec := &node.MetricsCapabilitySpec{}
	if err := protojson.Unmarshal([]byte(input), spec); err != nil {
		return nil, err
	}
	return spec, nil
}
