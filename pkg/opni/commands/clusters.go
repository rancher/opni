package commands

import (
	"fmt"
	"reflect"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	cliutil "github.com/rancher/opni/pkg/opni/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
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
	ConfigureManagementCommand(clustersCmd)
	return clustersCmd
}

func BuildClustersListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List clusters",
		Run: func(cmd *cobra.Command, args []string) {
			t, err := mgmtClient.ListClusters(cmd.Context(), &managementv1.ListClustersRequest{})
			if err != nil {
				lg.Fatal(err)
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
					zap.Error(err),
				).Warn("failed to query cortex stats")
			}
			clusterStats = stats
			fmt.Println(cliutil.RenderClusterList(t, healthStatus, clusterStats))
		},
	}
	return cmd
}

func BuildClustersDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <cluster-id> [<cluster-id> ...]",
		Aliases: []string{"rm"},
		Short:   "Delete a cluster",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, cluster := range args {
				_, err := mgmtClient.DeleteCluster(cmd.Context(), &corev1.Reference{
					Id: cluster,
				})
				if err != nil {
					lg.Fatal(err)
				}
				lg.With(
					"id", cluster,
				).Info("Deleted cluster")
			}
			return nil
		},
	}
	return cmd
}

func BuildClustersLabelCmd() *cobra.Command {
	overwrite := false
	cmd := &cobra.Command{
		Use:   "label [--overwrite] <cluster-id> <label>=<value> [<label>=<value>...]",
		Short: "Add labels to a cluster",
		Args:  cobra.MinimumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			clusterID := args[0]
			cluster, err := mgmtClient.GetCluster(cmd.Context(), &corev1.Reference{
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
			for k, v := range cluster.GetLabels() {
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
			updatedCluster, err := mgmtClient.EditCluster(cmd.Context(), &managementv1.EditClusterRequest{
				Cluster: cluster.Reference(),
				Labels:  currentLabels,
			})
			if err != nil {
				lg.With(
					zap.Error(err),
				).Fatal("Failed to edit cluster")
			}
			if reflect.DeepEqual(cluster.GetLabels(), updatedCluster.GetLabels()) {
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
