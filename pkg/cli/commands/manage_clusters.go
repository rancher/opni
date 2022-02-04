package commands

import (
	"fmt"
	"reflect"

	cliutil "github.com/rancher/opni-monitoring/pkg/cli/util"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func BuildClustersCmd() *cobra.Command {
	clustersCmd := &cobra.Command{
		Use:     "clusters",
		Aliases: []string{"cluster"},
		Short:   "Manage clusters",
	}
	clustersCmd.AddCommand(BuildClustersListCmd())
	clustersCmd.AddCommand(BuildClustersDeleteCmd())
	clustersCmd.AddCommand(BuildClustersLabelCmd())
	return clustersCmd
}

func BuildClustersListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List clusters",
		Run: func(cmd *cobra.Command, args []string) {
			t, err := client.ListClusters(cmd.Context(), &management.ListClustersRequest{})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderClusterList(t))
		},
	}
}

func BuildClustersDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <cluster-id> [<cluster-id>...]",
		Aliases: []string{"rm"},
		Short:   "Delete a cluster",
		Args:    cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			for _, cluster := range args {
				_, err := client.DeleteCluster(cmd.Context(),
					&core.Reference{
						Id: cluster,
					},
				)
				if err != nil {
					lg.Fatal(err)
				}
				lg.With(
					"id", cluster,
				).Info("Deleted cluster")
			}
		},
	}
}

func BuildClustersLabelCmd() *cobra.Command {
	overwrite := false
	cmd := &cobra.Command{
		Use:   "label [--overwrite] <cluster-id> <label>=<value> [<label>=<value>...]",
		Short: "Add labels to a cluster",
		Args:  cobra.MinimumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			clusterID := args[0]
			cluster, err := client.GetCluster(cmd.Context(), &core.Reference{
				Id: clusterID,
			})
			if err != nil {
				lg.Fatal(err)
			}
			labels, err := cliutil.ParseKeyValuePairs(args[1:])
			if err != nil {
				lg.Fatal(err)
			}
			currentLabels := map[string]string{}
			for k, v := range cluster.Labels {
				currentLabels[k] = v
			}
			for k, v := range labels {
				if v == "-" {
					delete(currentLabels, k)
					continue
				}
				if overwrite {
					currentLabels[k] = v
				} else {
					if _, ok := currentLabels[k]; !ok {
						currentLabels[k] = v
					} else {
						lg.With(
							"key", k,
							"curValue", v,
						).Fatal("Label already exists (use --overwrite to enable replacing existing values)")
					}
				}
			}
			updatedCluster, err := client.EditCluster(cmd.Context(), &management.EditClusterRequest{
				Cluster: cluster.Reference(),
				Labels:  currentLabels,
			})
			if err != nil {
				lg.With(
					zap.Error(err),
				).Fatal("Failed to edit cluster")
			}
			if reflect.DeepEqual(cluster.Labels, updatedCluster.Labels) {
				lg.Error("Updating cluster labels failed (unknown error)")
				return
			}
			lg.With(
				"id", clusterID,
			).Info("Cluster labels updated")
		},
	}
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Enable overwriting existing label values")
	return cmd
}
