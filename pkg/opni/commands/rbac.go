//go:build !minimal

package commands

import (
	"fmt"
	"os"
	"sort"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/opni/cliutil"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
)

func BuildRolesCmd() *cobra.Command {
	rolesCmd := &cobra.Command{
		Use:     "roles",
		Aliases: []string{"role"},
		Short:   "Manage roles",
	}
	rolesCmd.AddCommand(BuildRolesCreateCmd())
	rolesCmd.AddCommand(BuildRolesUpdateCmd())
	rolesCmd.AddCommand(BuildRolesDeleteCmd())
	rolesCmd.AddCommand(BuildRolesShowCmd())
	rolesCmd.AddCommand(BuildRolesListCmd())
	ConfigureManagementCommand(rolesCmd)
	return rolesCmd
}

func BuildRoleBindingsCmd() *cobra.Command {
	roleBindingsCmd := &cobra.Command{
		Use:     "rolebindings",
		Aliases: []string{"rb", "rolebinding"},
		Short:   "Manage role bindings",
	}
	roleBindingsCmd.AddCommand(BuildRoleBindingsCreateCmd())
	roleBindingsCmd.AddCommand(BuildRoleBindingsUpdateCmd())
	roleBindingsCmd.AddCommand(BuildRoleBindingsDeleteCmd())
	roleBindingsCmd.AddCommand(BuildRoleBindingsShowCmd())
	roleBindingsCmd.AddCommand(BuildRoleBindingsListCmd())
	ConfigureManagementCommand(roleBindingsCmd)
	return roleBindingsCmd
}

func BuildRolesCreateCmd() *cobra.Command {
	var clusterIDs []string
	var matchLabelsStrings []string
	matchLabels := map[string]string{}
	cmd := &cobra.Command{
		Use:               "create <role-id>",
		Short:             "Create a role",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// split key=value strings in matchLabels
			var err error
			matchLabels, err = cliutil.ParseKeyValuePairs(matchLabelsStrings)
			if err != nil {
				return err
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			role := &corev1.Role{
				Id:         args[0],
				ClusterIDs: clusterIDs,
				MatchLabels: &corev1.LabelSelector{
					MatchLabels: matchLabels,
				},
			}
			_, err := mgmtClient.CreateRole(cmd.Context(), role)
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			}
			fmt.Println(cliutil.RenderRole(role))
		},
	}
	cmd.Flags().StringSliceVar(&clusterIDs, "cluster-ids", []string{}, "Explicit cluster IDs to allow")
	cmd.Flags().StringSliceVar(&matchLabelsStrings, "match-labels", []string{}, "List of key=value cluster labels to match allowed clusters")
	return cmd
}

func BuildRolesUpdateCmd() *cobra.Command {
	var clusterIDs []string
	var matchLabelsStrings []string
	matchLabels := map[string]string{}
	cmd := &cobra.Command{
		Use:   "update <role-id>",
		Short: "Update a role",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeRoles(cmd, args, toComplete)
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// split key=value strings in matchLabels
			var err error
			matchLabels, err = cliutil.ParseKeyValuePairs(matchLabelsStrings)
			if err != nil {
				return err
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			role := &corev1.Role{
				Id:         args[0],
				ClusterIDs: clusterIDs,
				MatchLabels: &corev1.LabelSelector{
					MatchLabels: matchLabels,
				},
			}
			_, err := mgmtClient.UpdateRole(cmd.Context(), role)
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			}
			fmt.Println(cliutil.RenderRole(role))
		},
	}
	cmd.Flags().StringSliceVar(&clusterIDs, "cluster-ids", []string{}, "Explicit cluster IDs to allow")
	cmd.Flags().StringSliceVar(&matchLabelsStrings, "match-labels", []string{}, "List of key=value cluster labels to match allowed clusters")
	return cmd
}

func BuildRolesDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <role-id> [<role-id>...]",
		Aliases: []string{"rm"},
		Short:   "Delete roles",
		Args:    cobra.MinimumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeRoles(cmd, args, toComplete)
		},
		Run: func(cmd *cobra.Command, args []string) {
			for _, role := range args {
				_, err := mgmtClient.DeleteRole(cmd.Context(),
					&corev1.Reference{
						Id: role,
					})
				if err != nil {
					lg.Error("fatal", logger.Err(err))
					os.Exit(1)
				}
				fmt.Println(role)
			}
		},
	}
}

func BuildRolesShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <role-id>",
		Short: "Show detailed information about a role",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeRoles(cmd, args, toComplete)
		},
		Run: func(cmd *cobra.Command, args []string) {
			role, err := mgmtClient.GetRole(cmd.Context(),
				&corev1.Reference{
					Id: args[0],
				})
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			}
			fmt.Println(cliutil.RenderRole(role))
		},
	}
}

func BuildRolesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List roles",
		Aliases: []string{"ls"},
		Run: func(cmd *cobra.Command, args []string) {
			t, err := mgmtClient.ListRoles(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			}
			fmt.Println(cliutil.RenderRoleList(t))
		},
	}
}

func BuildRoleBindingsCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <rolebinding-id> <role-id> <user-id>...",
		Short: "Create a role binding",
		Args:  cobra.MinimumNArgs(3),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 1 {
				return completeRoles(cmd, args, toComplete)
			}

			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		Run: func(cmd *cobra.Command, args []string) {
			rb := &corev1.RoleBinding{
				Id:       args[0],
				RoleId:   args[1],
				Subjects: args[2:],
			}
			_, err := mgmtClient.CreateRoleBinding(cmd.Context(), rb)
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			}
			rb, err = mgmtClient.GetRoleBinding(cmd.Context(), rb.Reference())
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			}
			fmt.Println(cliutil.RenderRoleBinding(rb))
		},
	}
}

func BuildRoleBindingsUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update <rolebinding-id> <role-id> <user-id>...",
		Short: "Update a role binding",
		Args:  cobra.MinimumNArgs(3),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			switch len(args) {
			case 0:
				return completeRoleBindings(cmd, args, toComplete)
			case 1:
				return completeRoles(cmd, args, toComplete)
			}

			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		Run: func(cmd *cobra.Command, args []string) {
			rb := &corev1.RoleBinding{
				Id:       args[0],
				RoleId:   args[1],
				Subjects: args[2:],
			}
			_, err := mgmtClient.UpdateRoleBinding(cmd.Context(), rb)
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			}
			rb, err = mgmtClient.GetRoleBinding(cmd.Context(), rb.Reference())
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			}
			fmt.Println(cliutil.RenderRoleBinding(rb))
		},
	}
}

func BuildRoleBindingsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <rolebinding-id> [<rolebinding-id>...]",
		Aliases: []string{"rm"},
		Short:   "Delete role bindings",
		Args:    cobra.MinimumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeRoleBindings(cmd, args, toComplete)
		},
		Run: func(cmd *cobra.Command, args []string) {
			for _, rb := range args {
				_, err := mgmtClient.DeleteRoleBinding(cmd.Context(),
					&corev1.Reference{
						Id: rb,
					})
				if err != nil {
					lg.Error("fatal", logger.Err(err))
					os.Exit(1)
				}
				fmt.Println(rb)
			}
		},
	}
}

func BuildRoleBindingsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <rolebinding-id>",
		Short: "Show detailed information about a role binding",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeRoleBindings(cmd, args, toComplete)
		},
		Run: func(cmd *cobra.Command, args []string) {
			rb, err := mgmtClient.GetRoleBinding(cmd.Context(),
				&corev1.Reference{
					Id: args[0],
				})
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			} else {
				fmt.Println(cliutil.RenderRoleBinding(rb))
			}
		},
	}
}

func BuildRoleBindingsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List role bindings",
		Aliases: []string{"ls"},
		Run: func(cmd *cobra.Command, args []string) {
			t, err := mgmtClient.ListRoleBindings(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			}
			fmt.Println(cliutil.RenderRoleBindingList(t))
		},
	}
}

func BuildAccessMatrixCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "access-matrix",
		Short: "Print an access matrix showing all users and their allowed clusters",
		Run: func(cmd *cobra.Command, args []string) {
			rbs, err := mgmtClient.ListRoleBindings(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				fmt.Println(err)
				return
			}
			allUsers := map[string]struct{}{}
			allClusters := map[string]struct{}{}
			clusterToUsers := make(map[string]map[string]struct{})
			for _, rb := range rbs.Items {
				for _, subject := range rb.Subjects {
					allUsers[subject] = struct{}{}
				}
			}
			clusters, err := mgmtClient.ListClusters(cmd.Context(), &managementv1.ListClustersRequest{})
			if err != nil {
				lg.Error("fatal", logger.Err(err))
				os.Exit(1)
			}
			for _, cluster := range clusters.Items {
				allClusters[cluster.Id] = struct{}{}
				clusterToUsers[cluster.Id] = make(map[string]struct{})
			}
			for user := range allUsers {
				clusterIds, err := mgmtClient.SubjectAccess(cmd.Context(),
					&corev1.SubjectAccessRequest{
						Subject: user,
					})
				if err != nil {
					lg.Error("fatal", logger.Err(err))
					os.Exit(1)
				}
				for _, ref := range clusterIds.Items {
					if _, ok := clusterToUsers[ref.Id]; !ok {
						clusterToUsers[ref.Id] = make(map[string]struct{})
					}
					clusterToUsers[ref.Id][user] = struct{}{}
				}
			}
			sortedUsers := make([]string, 0, len(allUsers))
			for user := range allUsers {
				sortedUsers = append(sortedUsers, user)
			}
			sort.Strings(sortedUsers)
			fmt.Println(cliutil.RenderAccessMatrix(cliutil.AccessMatrix{
				Users:           sortedUsers,
				KnownClusters:   allClusters,
				ClustersToUsers: clusterToUsers,
			}))
		},
	}
	ConfigureManagementCommand(cmd)
	return cmd
}

func init() {
	AddCommandsToGroup(ManagementAPI, BuildRolesCmd(), BuildRoleBindingsCmd())
	AddCommandsToGroup(Utilities, BuildAccessMatrixCmd())
}
