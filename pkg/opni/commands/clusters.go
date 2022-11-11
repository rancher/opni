package commands

import (
	"fmt"
	"reflect"

	tea "github.com/charmbracelet/bubbletea"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/opni/ui"
	cliutil "github.com/rancher/opni/pkg/opni/util"
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
	clustersCmd.AddCommand(BuildClustersWatchCmd())
	clustersCmd.AddCommand(BuildClustersDeleteCmd())
	clustersCmd.AddCommand(BuildClustersLabelCmd())
	clustersCmd.AddCommand(BuildClustersRenameCmd())
	ConfigureManagementCommand(clustersCmd)
	return clustersCmd
}

func BuildClustersListCmd() *cobra.Command {
	var watch bool
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List clusters",
		RunE: func(cmd *cobra.Command, args []string) error {
			if watch {
				m := ui.NewClusterListModel()
				w := &ui.ClusterListWatcher{
					Messages: make(chan tea.Msg, 100),
					Client:   mgmtClient,
				}
				go func() {
					if err := w.Run(cmd.Context()); err != nil {
						lg.Fatal(err)
					}
				}()
				p := tea.NewProgram(m)
				go func() {
					for {
						select {
						case msg := <-w.Messages:
							p.Send(msg)
						case <-cmd.Context().Done():
							p.Send(tea.Quit())
							return
						}
					}
				}()
				return p.Start()
			} else {
				t, err := mgmtClient.ListClusters(cmd.Context(), &managementv1.ListClustersRequest{})
				if err != nil {
					return err
				}
				var healthStatus []*corev1.HealthStatus
				for _, c := range t.Items {
					stat, err := mgmtClient.GetClusterHealthStatus(cmd.Context(), c.Reference())
					if err != nil {
						healthStatus = append(healthStatus, &corev1.HealthStatus{})
					} else {
						healthStatus = append(healthStatus, stat)
					}
				}
				fmt.Println(cliutil.RenderClusterList(t, healthStatus))
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "Watch for updates")
	return cmd
}

func BuildClustersWatchCmd() *cobra.Command {
	cmd := BuildClustersListCmd()
	cmd.Use = "watch"
	cmd.Aliases = []string{}
	cmd.Short = "Alias for 'list --watch'"
	cmd.Flags().Set("watch", "true")
	cmd.Flags().MarkHidden("watch")
	return cmd
}

func BuildClustersDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <cluster-id> [<cluster-id> ...]",
		Aliases: []string{"rm"},
		Short:   "Delete a cluster",
		Args:    cobra.MinimumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeClusters(cmd, args, toComplete)
		},
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
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return completeClusters(cmd, args, toComplete)
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
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

func BuildClustersRenameCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename <cluster-id> <new-name>",
		Short: "Rename a cluster",
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return completeClusters(cmd, args, toComplete)
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := mgmtClient.GetCluster(cmd.Context(), &corev1.Reference{
				Id: args[0],
			})
			if err != nil {
				return err
			}
			labels := cluster.GetLabels()
			if labels == nil {
				labels = map[string]string{}
			}
			oldName, hasOldName := labels[corev1.NameLabel]
			labels[corev1.NameLabel] = args[1]
			if _, err := mgmtClient.EditCluster(cmd.Context(), &managementv1.EditClusterRequest{
				Cluster: &corev1.Reference{
					Id: args[0],
				},
				Labels: labels,
			}); err != nil {
				return err
			}
			lg := lg.With(
				"id", args[0],
				"newName", args[1],
			)
			if hasOldName {
				lg = lg.With(
					"oldName", oldName,
				)
			}
			lg.Info("Renamed cluster")
			return nil
		},
	}
	return cmd
}

func init() {
	AddCommandsToGroup(ManagementAPI, BuildClustersCmd())
}
