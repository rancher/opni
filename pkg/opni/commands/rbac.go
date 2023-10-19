//go:build !minimal

package commands

import (
	"fmt"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/opni/cliutil"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
)

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

func BuildRoleBindingsCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <rolebinding-id> <role-id> <user-id>...",
		Short: "Create a role binding",
		Args:  cobra.MinimumNArgs(3),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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
				lg.Fatal(err)
			}
			rb, err = mgmtClient.GetRoleBinding(cmd.Context(), rb.Reference())
			if err != nil {
				lg.Fatal(err)
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
			if len(args) == 0 {
				return completeRoleBindings(cmd, args, toComplete)
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
				lg.Fatal(err)
			}
			rb, err = mgmtClient.GetRoleBinding(cmd.Context(), rb.Reference())
			if err != nil {
				lg.Fatal(err)
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
					lg.Fatal(err)
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
				lg.Fatal(err)
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
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderRoleBindingList(t))
		},
	}
}

func init() {
	AddCommandsToGroup(ManagementAPI, BuildRoleBindingsCmd())
	AddCommandsToGroup(ManagementAPI, managementv1.BuildManagementListRBACBackendsCmd())
	AddCommandsToGroup(ManagementAPI, managementv1.BuildManagementGetAvailableBackendPermissionsCmd())
	AddCommandsToGroup(ManagementAPI, managementv1.BuildManagementCreateBackendRoleCmd())
	AddCommandsToGroup(ManagementAPI, managementv1.BuildManagementUpdateBackendRoleCmd())
	AddCommandsToGroup(ManagementAPI, managementv1.BuildManagementDeleteBackendRoleCmd())
	AddCommandsToGroup(ManagementAPI, managementv1.BuildManagementGetBackendRoleCmd())
	AddCommandsToGroup(ManagementAPI, managementv1.BuildManagementListBackendRolesCmd())
}
