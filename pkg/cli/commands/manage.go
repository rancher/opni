package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	cliutil "github.com/kralicky/opni-gateway/pkg/cli/util"
	"github.com/kralicky/opni-gateway/pkg/management"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/durationpb"
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
	manageCmd.AddCommand(BuildCertsCmd())
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
				cliutil.RenderBootstrapTokenList(t)
			}
			return nil
		},
	}
	tokensCreateCmd.Flags().String("ttl", management.DefaultTokenTTL.String(), "Time to live")
	return tokensCreateCmd
}

func BuildTokensRevokeCmd() *cobra.Command {
	tokensRevokeCmd := &cobra.Command{
		Use:   "revoke",
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
				cliutil.RenderBootstrapTokenList(t.Tokens...)
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
			for range t.Tenants {
				// todo
			}
		},
	}
	return tenantsListCmd
}

func printTable(tokens ...*management.BootstrapToken) {
	w := table.NewWriter()
	w.SetStyle(table.StyleColoredDark)
	w.AppendHeader(table.Row{"ID", "TOKEN", "TTL"})
	for _, t := range tokens {
		token := t.ToToken()
		w.AppendRow(table.Row{token.HexID(), token.EncodeHex(), t.GetTTL()})
	}
	fmt.Println(w.Render())
}
