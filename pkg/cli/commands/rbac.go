package commands

import (
	"fmt"
	"sort"

	cliutil "github.com/rancher/opni-monitoring/pkg/cli/util"
	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/management"
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
		Use:   "create <role-id>",
		Short: "Create a role",
		Args:  cobra.ExactArgs(1),
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
			role := &core.Role{
				Id:         args[0],
				ClusterIDs: clusterIDs,
				MatchLabels: &core.LabelSelector{
					MatchLabels: matchLabels,
				},
			}
			_, err := client.CreateRole(cmd.Context(), role)
			if err != nil {
				lg.Fatal(err)
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
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			for _, role := range args {
				_, err := client.DeleteRole(cmd.Context(),
					&core.Reference{
						Id: role,
					})
				if err != nil {
					lg.Fatal(err)
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
		Run: func(cmd *cobra.Command, args []string) {
			role, err := client.GetRole(cmd.Context(),
				&core.Reference{
					Id: args[0],
				})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderRole(role))
		},
	}
}

func BuildRolesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List roles",
		Run: func(cmd *cobra.Command, args []string) {
			t, err := client.ListRoles(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				lg.Fatal(err)
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
		Run: func(cmd *cobra.Command, args []string) {
			rb := &core.RoleBinding{
				Id:       args[0],
				RoleId:   args[1],
				Subjects: args[2:],
			}
			_, err := client.CreateRoleBinding(cmd.Context(), rb)
			if err != nil {
				lg.Fatal(err)
			}
			rb, err = client.GetRoleBinding(cmd.Context(), rb.Reference())
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderRoleBinding(rb))
		},
	}
}

func BuildRoleBindingsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <rolebinding-id>",
		Aliases: []string{"rm"},
		Short:   "Delete a role binding",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			_, err := client.DeleteRoleBinding(cmd.Context(),
				&core.Reference{
					Id: args[0],
				})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(args[0])
		},
	}
}

func BuildRoleBindingsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <rolebinding-id>",
		Short: "Show detailed information about a role binding",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			rb, err := client.GetRoleBinding(cmd.Context(),
				&core.Reference{
					Id: args[0],
				})
			if err != nil {
				lg.Fatal(err)
			} else {
				fmt.Println(cliutil.RenderRoleBinding(rb))
			}
		},
	}
}

func BuildRoleBindingsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List role bindings",
		Run: func(cmd *cobra.Command, args []string) {
			t, err := client.ListRoleBindings(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				lg.Fatal(err)
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
			rbs, err := client.ListRoleBindings(cmd.Context(), &emptypb.Empty{})
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
			clusters, err := client.ListClusters(cmd.Context(), &management.ListClustersRequest{})
			if err != nil {
				lg.Fatal(err)
			}
			for _, cluster := range clusters.Items {
				allClusters[cluster.Id] = struct{}{}
				clusterToUsers[cluster.Id] = make(map[string]struct{})
			}
			for user := range allUsers {
				clusterIds, err := client.SubjectAccess(cmd.Context(),
					&core.SubjectAccessRequest{
						Subject: user,
					})
				if err != nil {
					lg.Fatal(err)
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
