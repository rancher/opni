package commands

import (
	"context"
	"fmt"
	"time"

	cliutil "github.com/kralicky/opni-monitoring/pkg/cli/util"
	"github.com/kralicky/opni-monitoring/pkg/management"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var client management.ManagementClient

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
	certsInfoCmd := &cobra.Command{
		Use:   "info",
		Short: "Show certificate information",
		Run: func(cmd *cobra.Command, args []string) {
			t, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
			if err != nil {
				fmt.Println(err)
				return
			}
			cliutil.RenderCertInfoChain(t.Chain)
		},
	}
	return certsInfoCmd
}

func BuildTokensCreateCmd() *cobra.Command {
	tokensCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a bootstrap token",
		RunE: func(cmd *cobra.Command, args []string) error {
			ttl := cmd.Flag("ttl").Value.String()
			duration, err := time.ParseDuration(ttl)
			if err != nil {
				return err
			}
			t, err := client.CreateBootstrapToken(context.Background(),
				&management.CreateBootstrapTokenRequest{
					TTL: durationpb.New(duration),
				})
			if err != nil {
				fmt.Println(err)
			} else {
				cliutil.RenderBootstrapTokenList([]*management.BootstrapToken{t})
			}
			return nil
		},
	}
	tokensCreateCmd.Flags().String("ttl", management.DefaultTokenTTL.String(), "Time to live")
	return tokensCreateCmd
}

func BuildTokensRevokeCmd() *cobra.Command {
	tokensRevokeCmd := &cobra.Command{
		Use:   "revoke <token>",
		Short: "Revoke a bootstrap token",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			for _, token := range args {
				_, err := client.RevokeBootstrapToken(context.Background(),
					&management.RevokeBootstrapTokenRequest{
						TokenID: token,
					})
				if err == nil {
					fmt.Printf("Revoked token %s\n", token)
				} else {
					fmt.Println(err)
				}
			}
		},
	}
	return tokensRevokeCmd
}

func BuildTokensListCmd() *cobra.Command {
	tokensListCmd := &cobra.Command{
		Use:   "list",
		Short: "List bootstrap tokens",
		Run: func(cmd *cobra.Command, args []string) {
			t, err := client.ListBootstrapTokens(context.Background(),
				&management.ListBootstrapTokensRequest{})
			if err != nil {
				fmt.Println(err)
			} else {
				cliutil.RenderBootstrapTokenList(t.Tokens)
			}
		},
	}
	return tokensListCmd
}

func BuildTenantsListCmd() *cobra.Command {
	tenantsListCmd := &cobra.Command{
		Use:   "list",
		Short: "List tenants",
		Run: func(cmd *cobra.Command, args []string) {
			t, err := client.ListTenants(context.Background(), &emptypb.Empty{})
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Println(cliutil.RenderTenantList(t.Tenants))
		},
	}
	return tenantsListCmd
}

func BuildTenantsDeleteCmd() *cobra.Command {
	tenantsDeleteCmd := &cobra.Command{
		Use:     "delete <tenant-id>",
		Aliases: []string{"rm"},
		Short:   "Delete a tenant",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			_, err := client.DeleteTenant(context.Background(),
				&management.Tenant{
					ID: args[0],
				},
			)
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Printf("Deleted tenant %s\n", args[0])
		},
	}
	return tenantsDeleteCmd
}

func BuildRolesCmd() *cobra.Command {
	rolesCmd := &cobra.Command{
		Use:     "roles",
		Aliases: []string{"role"},
		Short:   "Manage roles",
	}
	rolesCmd.AddCommand(BuildRolesCreateCmd())
	rolesCmd.AddCommand(BuildRolesDeleteCmd())
	rolesCmd.AddCommand(BuildRolesGetCmd())
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
	roleBindingsCmd.AddCommand(BuildRoleBindingsGetCmd())
	roleBindingsCmd.AddCommand(BuildRoleBindingsListCmd())
	return roleBindingsCmd
}

func BuildRolesCreateCmd() *cobra.Command {
	rolesCreateCmd := &cobra.Command{
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
				fmt.Println(err)
			} else {
				fmt.Println(cliutil.RenderRole(newRole))
			}
		},
	}
	return rolesCreateCmd
}

func BuildRolesDeleteCmd() *cobra.Command {
	rolesDeleteCmd := &cobra.Command{
		Use:     "delete <name>",
		Aliases: []string{"rm"},
		Short:   "Delete a role",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			for _, role := range args {
				_, err := client.DeleteRole(context.Background(),
					&management.DeleteRoleRequest{
						Name: role,
					})
				if err != nil {
					fmt.Println(err)
				} else {
					fmt.Printf("Deleted role %s\n", role)
				}
			}
		},
	}
	return rolesDeleteCmd
}

func BuildRolesGetCmd() *cobra.Command {
	rolesGetCmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Get detailed information about a role",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			role, err := client.GetRole(context.Background(),
				&management.GetRoleRequest{
					Name: args[0],
				})
			if err != nil {
				fmt.Println(err)
			} else {
				fmt.Println(cliutil.RenderRole(role))
			}
		},
	}
	return rolesGetCmd
}

func BuildRolesListCmd() *cobra.Command {
	rolesListCmd := &cobra.Command{
		Use:   "list",
		Short: "List roles",
		Run: func(cmd *cobra.Command, args []string) {
			t, err := client.ListRoles(context.Background(), &emptypb.Empty{})
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Println(cliutil.RenderRoleList(t.Items))
		},
	}
	return rolesListCmd
}

func BuildRoleBindingsCreateCmd() *cobra.Command {
	roleBindingsCreateCmd := &cobra.Command{
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
				fmt.Println(err)
			} else {
				fmt.Println(cliutil.RenderRoleBinding(rb))
			}
		},
	}
	return roleBindingsCreateCmd
}

func BuildRoleBindingsDeleteCmd() *cobra.Command {
	roleBindingsDeleteCmd := &cobra.Command{
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
				fmt.Println(err)
			} else {
				fmt.Printf("Deleted role binding %s\n", args[0])
			}
		},
	}
	return roleBindingsDeleteCmd
}

func BuildRoleBindingsGetCmd() *cobra.Command {
	roleBindingsGetCmd := &cobra.Command{
		Use:   "get <rolebinding-name>",
		Short: "Get detailed information about a role binding",
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
	return roleBindingsGetCmd
}

func BuildRoleBindingsListCmd() *cobra.Command {
	roleBindingsListCmd := &cobra.Command{
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
	return roleBindingsListCmd
}
