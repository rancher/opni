package commands

import (
	"context"
	"fmt"
	"sort"
	"time"

	cliutil "github.com/kralicky/opni-monitoring/pkg/cli/util"
	"github.com/kralicky/opni-monitoring/pkg/logger"
	"github.com/kralicky/opni-monitoring/pkg/management"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var client management.ManagementClient
var lg = logger.New().Named("management")

func BuildManageCmd() *cobra.Command {
	var address string
	manageCmd := &cobra.Command{
		Use:   "manage",
		Short: "Interact with the gateway's management API",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			c, err := management.NewClient(context.Background(),
				management.WithListenAddress(address))
			if err != nil {
				return err
			}
			client = c
			return nil
		},
	}
	manageCmd.PersistentFlags().StringVarP(&address, "address", "a",
		management.DefaultManagementSocket(), "Management API address")
	manageCmd.AddCommand(BuildTokensCmd())
	manageCmd.AddCommand(BuildTenantsCmd())
	manageCmd.AddCommand(BuildCertsCmd())
	manageCmd.AddCommand(BuildRolesCmd())
	manageCmd.AddCommand(BuildRoleBindingsCmd())
	manageCmd.AddCommand(BuildAccessMatrixCmd())
	return manageCmd
}

func BuildTokensCmd() *cobra.Command {
	tokensCmd := &cobra.Command{
		Use:   "tokens",
		Short: "Manage bootstrap tokens",
	}
	tokensCmd.AddCommand(BuildTokensCreateCmd())
	tokensCmd.AddCommand(BuildTokensRevokeCmd())
	tokensCmd.AddCommand(BuildTokensListCmd())
	return tokensCmd
}

func BuildTenantsCmd() *cobra.Command {
	tenantsCmd := &cobra.Command{
		Use:   "tenants",
		Short: "Manage tenants",
	}
	tenantsCmd.AddCommand(BuildTenantsListCmd())
	tenantsCmd.AddCommand(BuildTenantsDeleteCmd())
	return tenantsCmd
}

func BuildCertsCmd() *cobra.Command {
	certsCmd := &cobra.Command{
		Use:   "certs",
		Short: "Manage certificates",
	}
	certsCmd.AddCommand(BuildCertsInfoCmd())
	return certsCmd
}

func BuildCertsInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show certificate information",
		Run: func(cmd *cobra.Command, args []string) {
			t, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderCertInfoChain(t.Chain))
		},
	}
}

func BuildTokensCreateCmd() *cobra.Command {
	tokensCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a bootstrap token",
		Run: func(cmd *cobra.Command, args []string) {
			ttl := cmd.Flag("ttl").Value.String()
			duration, err := time.ParseDuration(ttl)
			if err != nil {
				lg.Fatal(err)
			}
			t, err := client.CreateBootstrapToken(context.Background(),
				&management.CreateBootstrapTokenRequest{
					TTL: durationpb.New(duration),
				})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderBootstrapTokenList([]*management.BootstrapToken{t}))
		},
	}
	tokensCreateCmd.Flags().String("ttl", management.DefaultTokenTTL.String(), "Time to live")
	return tokensCreateCmd
}

func BuildTokensRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <token>",
		Short: "Revoke a bootstrap token",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			for _, token := range args {
				_, err := client.RevokeBootstrapToken(context.Background(),
					&management.RevokeBootstrapTokenRequest{
						TokenID: token,
					})
				if err != nil {
					lg.Fatal(err)
				}
				lg.Info("Revoked token %s\n", token)
			}
		},
	}
}

func BuildTokensListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List bootstrap tokens",
		Run: func(cmd *cobra.Command, args []string) {
			t, err := client.ListBootstrapTokens(context.Background(),
				&management.ListBootstrapTokensRequest{})
			if err != nil {
				lg.Fatal(err)
			}
			cliutil.RenderBootstrapTokenList(t.Tokens)
		},
	}
}

func BuildTenantsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List tenants",
		Run: func(cmd *cobra.Command, args []string) {
			t, err := client.ListTenants(context.Background(), &emptypb.Empty{})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderClusterList(t.Tenants))
		},
	}
}

func BuildTenantsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <tenant-id> [<tenant-id>...]",
		Aliases: []string{"rm"},
		Short:   "Delete a tenant",
		Args:    cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			for _, tenant := range args {
				_, err := client.DeleteTenant(context.Background(),
					&management.Tenant{
						ID: tenant,
					},
				)
				if err != nil {
					lg.Fatal(err)
				}
				lg.With(
					"id", tenant,
				).Info("Deleted tenant")
			}
		},
	}
}

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
	return roleBindingsCmd
}

func BuildRolesCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create name tenant-id [...,tenant-id]",
		Short: "Create a role",
		Args:  cobra.MinimumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			newRole, err := client.CreateRole(context.Background(),
				&management.CreateRoleRequest{
					Role: &management.Role{
						Name:      args[0],
						TenantIDs: args[1:],
					},
				})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderRole(newRole))
		},
	}
}

func BuildRolesDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <role-id> [<role-id>...]",
		Aliases: []string{"rm"},
		Short:   "Delete roles",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			for _, role := range args {
				_, err := client.DeleteRole(context.Background(),
					&management.DeleteRoleRequest{
						Name: role,
					})
				if err != nil {
					lg.Fatal(err)
				}
				fmt.Printf("Deleted role %s\n", role)
			}
		},
	}
}

func BuildRolesShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show detailed information about a role",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			role, err := client.GetRole(context.Background(),
				&management.GetRoleRequest{
					Name: args[0],
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
			t, err := client.ListRoles(context.Background(), &emptypb.Empty{})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderRoleList(t.Items))
		},
	}
}

func BuildRoleBindingsCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <rolebinding-name> <role-name> <user-id>",
		Short: "Create a role binding",
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			rb, err := client.CreateRoleBinding(context.Background(),
				&management.CreateRoleBindingRequest{
					RoleBinding: &management.RoleBinding{
						Name:     args[0],
						RoleName: args[1],
						UserID:   args[2],
					},
				})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderRoleBinding(rb))
		},
	}
}

func BuildRoleBindingsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <rolebinding-name>",
		Aliases: []string{"rm"},
		Short:   "Delete a role binding",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			_, err := client.DeleteRoleBinding(context.Background(),
				&management.DeleteRoleBindingRequest{
					Name: args[0],
				})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Printf("Deleted role binding %s\n", args[0])
		},
	}
}

func BuildRoleBindingsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <rolebinding-name>",
		Short: "Show detailed information about a role binding",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			rb, err := client.GetRoleBinding(context.Background(),
				&management.GetRoleBindingRequest{
					Name: args[0],
				})
			if err != nil {
				fmt.Println(err)
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
			t, err := client.ListRoleBindings(context.Background(), &emptypb.Empty{})
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Println(cliutil.RenderRoleBindingList(t.Items))
		},
	}
}

func BuildAccessMatrixCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "access-matrix",
		Short: "Print an access matrix showing all users and their allowed tenants",
		Run: func(cmd *cobra.Command, args []string) {
			rbs, err := client.ListRoleBindings(context.Background(), &emptypb.Empty{})
			if err != nil {
				fmt.Println(err)
				return
			}
			users := map[string]struct{}{}
			tenantsToUsers := make(map[string]map[string]struct{})
			for _, rb := range rbs.Items {
				role, err := client.GetRole(context.Background(),
					&management.GetRoleRequest{
						Name: rb.RoleName,
					})
				if err != nil {
					lg.Fatal(err)
				}
				users[rb.UserID] = struct{}{}
				for _, tenantID := range role.TenantIDs {
					if m, ok := tenantsToUsers[tenantID]; !ok {
						tenantsToUsers[tenantID] = map[string]struct{}{rb.UserID: struct{}{}}
					} else {
						m[rb.UserID] = struct{}{}
					}
				}
			}
			sortedUsers := make([]string, 0, len(users))
			for user := range users {
				sortedUsers = append(sortedUsers, user)
			}
			sort.Strings(sortedUsers)
			fmt.Println(cliutil.RenderAccessMatrix(cliutil.AccessMatrix{
				Users:          sortedUsers,
				TenantsToUsers: tenantsToUsers,
			}))
		},
	}
}
