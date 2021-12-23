package commands

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/kralicky/opni-gateway/pkg/management"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"
)

var client management.ManagementClient

func BuildManageCmd() *cobra.Command {
	var socket string
	manageCmd := &cobra.Command{
		Use:   "manage",
		Short: "Interact with the gateway's management API",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			c, err := management.NewClient(management.WithSocket(socket))
			if err != nil {
				return err
			}
			client = c
			return nil
		},
	}
	manageCmd.PersistentFlags().StringVar(&socket, "socket",
		management.DefaultManagementSocket, "Path to the management socket")
	manageCmd.AddCommand(BuildTokensCmd())
	manageCmd.AddCommand(BuildTenantsCmd())
	return manageCmd
}

func BuildTokensCmd() *cobra.Command {
	tokensCmd := &cobra.Command{
		Use:   "tokens",
		Short: "Manage bootstrap tokens",
	}
	tokensCmd.AddCommand(BuildTokensCreateCmd())
	tokensCmd.AddCommand(BuildTokensListCmd())
	return tokensCmd
}

func BuildTenantsCmd() *cobra.Command {
	tenantsCmd := &cobra.Command{
		Use:   "tenants",
		Short: "Manage tenants",
	}
	tenantsCmd.AddCommand(BuildTenantsListCmd())
	return tenantsCmd
}

func BuildTokensCreateCmd() *cobra.Command {
	tokensCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a bootstrap token",
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := client.CreateBootstrapToken(context.Background(),
				&management.CreateBootstrapTokenRequest{})
			if err != nil {
				return err
			}
			printTable(t)
			return nil
		},
	}
	return tokensCreateCmd
}

func BuildTokensListCmd() *cobra.Command {
	tokensListCmd := &cobra.Command{
		Use:   "list",
		Short: "List bootstrap tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := client.ListBootstrapTokens(context.Background(),
				&management.ListBootstrapTokensRequest{})
			if err != nil {
				return err
			}
			printTable(t.Tokens...)
			return nil
		},
	}
	return tokensListCmd
}

func BuildTenantsListCmd() *cobra.Command {
	tenantsListCmd := &cobra.Command{
		Use:   "list",
		Short: "List tenants",
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := client.ListTenants(context.Background(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			for range t.Tenants {
				// todo
			}
			return nil
		},
	}
	return tenantsListCmd
}

func printTable(tokens ...*management.BootstrapToken) {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	w.AppendHeader(table.Row{"ID", "TOKEN", "EXPIRATION"})
	for _, t := range tokens {
		token := t.ToToken()
		w.AppendRow(table.Row{token.HexID(), token.EncodeHex(), t.GetExpiration()})
	}
	fmt.Println(w.Render())
}
