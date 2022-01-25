package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	cliutil "github.com/kralicky/opni-monitoring/pkg/cli/util"
	"github.com/kralicky/opni-monitoring/pkg/core"
	"github.com/kralicky/opni-monitoring/pkg/logger"
	"github.com/kralicky/opni-monitoring/pkg/management"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/durationpb"
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
	manageCmd.AddCommand(BuildClustersCmd())
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

func BuildClustersCmd() *cobra.Command {
	clustersCmd := &cobra.Command{
		Use:   "clusters",
		Short: "Manage clusters",
	}
	clustersCmd.AddCommand(BuildClustersListCmd())
	clustersCmd.AddCommand(BuildClustersDeleteCmd())
	return clustersCmd
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
			t, err := client.CertsInfo(context.Background())
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
					Ttl: durationpb.New(duration),
				})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderBootstrapTokenList(&core.BootstrapTokenList{
				Items: []*core.BootstrapToken{t},
			}))
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
				err := client.RevokeBootstrapToken(context.Background(),
					&core.Reference{
						Id: token,
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
			t, err := client.ListBootstrapTokens(context.Background())
			if err != nil {
				lg.Fatal(err)
			}
			cliutil.RenderBootstrapTokenList(t)
		},
	}
}

func BuildClustersListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List clusters",
		Run: func(cmd *cobra.Command, args []string) {
			t, err := client.ListClusters(context.Background(), &management.ListClustersRequest{})
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
				err := client.DeleteCluster(context.Background(),
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
	var clusterIDs []string
	var matchLabelsStrings []string
	matchLabels := map[string]string{}
	cmd := &cobra.Command{
		Use:   "create <role-name>",
		Short: "Create a role",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// split key=value strings in matchLabels
			for _, label := range matchLabelsStrings {
				kv := strings.SplitN(label, "=", 2)
				if len(kv) != 2 {
					return fmt.Errorf("invalid syntax: %q", label)
				}
				matchLabels[kv[0]] = kv[1]
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			role := &core.Role{
				Name:       args[0],
				ClusterIDs: clusterIDs,
				MatchLabels: &core.LabelSelector{
					MatchLabels: matchLabels,
				},
			}
			err := client.CreateRole(context.Background(), role)
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
				err := client.DeleteRole(context.Background(),
					&core.Reference{
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
				&core.Reference{
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
			t, err := client.ListRoles(context.Background())
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderRoleList(t))
		},
	}
}

func BuildRoleBindingsCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <rolebinding-name> <role-name> <user-id>...",
		Short: "Create a role binding",
		Args:  cobra.MaximumNArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			rb := &core.RoleBinding{
				Name:     args[0],
				RoleName: args[1],
				Subjects: args[2:],
			}
			err := client.CreateRoleBinding(context.Background(), rb)
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
			err := client.DeleteRoleBinding(context.Background(),
				&core.Reference{
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
				&core.Reference{
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
			t, err := client.ListRoleBindings(context.Background())
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Println(cliutil.RenderRoleBindingList(t))
		},
	}
}

func BuildAccessMatrixCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "access-matrix",
		Short: "Print an access matrix showing all users and their allowed clusters",
		Run: func(cmd *cobra.Command, args []string) {
			rbs, err := client.ListRoleBindings(context.Background())
			if err != nil {
				fmt.Println(err)
				return
			}
			users := map[string]struct{}{}
			clusterToUsers := make(map[string]map[string]struct{})
			for _, rb := range rbs.Items {
				for _, subject := range rb.Subjects {
					users[subject] = struct{}{}
				}
			}
			for user := range users {
				clusterIds, err := client.SubjectAccess(context.Background(),
					&core.SubjectAccessRequest{
						Subject: user,
					})
				if err != nil {
					fmt.Println(err)
					return
				}
				for _, ref := range clusterIds.Items {
					if _, ok := clusterToUsers[ref.Id]; !ok {
						clusterToUsers[ref.Id] = make(map[string]struct{})
					}
					clusterToUsers[ref.Id][user] = struct{}{}
				}
			}
			sortedUsers := make([]string, 0, len(users))
			for user := range users {
				sortedUsers = append(sortedUsers, user)
			}
			sort.Strings(sortedUsers)
			fmt.Println(cliutil.RenderAccessMatrix(cliutil.AccessMatrix{
				Users:           sortedUsers,
				ClustersToUsers: clusterToUsers,
			}))
		},
	}
}
