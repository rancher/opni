package commands

import (
	"fmt"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	cliutil "github.com/rancher/opni/pkg/opni/util"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

func BuildTokensCmd() *cobra.Command {
	tokensCmd := &cobra.Command{
		Use:     "tokens",
		Aliases: []string{"token"},
		Short:   "Manage bootstrap tokens",
	}
	tokensCmd.AddCommand(BuildTokensCreateCmd())
	tokensCmd.AddCommand(BuildTokensRevokeCmd())
	tokensCmd.AddCommand(BuildTokensListCmd())
	tokensCmd.AddCommand(BuildTokensGetCmd())
	ConfigureManagementCommand(tokensCmd)
	return tokensCmd
}

func BuildTokensCreateCmd() *cobra.Command {
	var ttl string
	var labels []string
	tokensCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a bootstrap token",
		Run: func(cmd *cobra.Command, args []string) {
			duration, err := time.ParseDuration(ttl)
			if err != nil {
				lg.Fatal(err)
			}
			labelMap, err := cliutil.ParseKeyValuePairs(labels)
			if err != nil {
				lg.Fatal(err)
			}
			t, err := mgmtClient.CreateBootstrapToken(cmd.Context(),
				&managementv1.CreateBootstrapTokenRequest{
					Ttl:    durationpb.New(duration),
					Labels: labelMap,
				})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderBootstrapToken(t))
		},
	}
	tokensCreateCmd.Flags().StringVar(&ttl, "ttl", "300s", "Time to live")
	tokensCreateCmd.Flags().StringSliceVar(&labels, "labels", []string{}, "Labels which will be auto-applied to any clusters created with this token")
	return tokensCreateCmd
}

func BuildTokensRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <token-id> [token-id]...",
		Short: "Revoke a bootstrap token",
		Args:  cobra.MinimumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeBootstrapTokens(cmd, args, toComplete)
		},
		Run: func(cmd *cobra.Command, args []string) {
			for _, token := range args {
				_, err := mgmtClient.RevokeBootstrapToken(cmd.Context(),
					&corev1.Reference{
						Id: token,
					})
				if err != nil {
					lg.Fatal(err)
				}
				lg.Infof("Revoked token %s", token)
			}
		},
	}
}

func BuildTokensListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List bootstrap tokens",
		Run: func(cmd *cobra.Command, args []string) {
			t, err := mgmtClient.ListBootstrapTokens(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(cliutil.RenderBootstrapTokenList(t))
		},
	}
}

func BuildTokensGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <token-id> [token-id]...",
		Short: "get bootstrap tokens",
		Args:  cobra.MinimumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeBootstrapTokens(cmd, args, toComplete)
		},
		Run: func(cmd *cobra.Command, args []string) {
			tokenList := []*corev1.BootstrapToken{}
			for _, id := range args {
				t, err := mgmtClient.GetBootstrapToken(cmd.Context(), &corev1.Reference{
					Id: id,
				})
				if err != nil {
					lg.Fatal(err)
				}
				tokenList = append(tokenList, t)
			}
			cliutil.RenderBootstrapTokenList(&corev1.BootstrapTokenList{
				Items: tokenList,
			})
		},
	}
}

func init() {
	AddCommandsToGroup(ManagementAPI, BuildTokensCmd())
}
