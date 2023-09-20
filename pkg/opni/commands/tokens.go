//go:build !minimal

package commands

import (
	"fmt"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/opni/cliutil"
	"github.com/rancher/opni/pkg/render"
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
	tokensCmd.AddCommand(BuildTokensCreateSupportCmd())
	ConfigureManagementCommand(tokensCmd)
	return tokensCmd
}

func BuildTokensCreateCmd() *cobra.Command {
	var ttl string
	var labels []string
	var maxUsages int
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
					Ttl:       durationpb.New(duration),
					Labels:    labelMap,
					MaxUsages: int64(maxUsages),
				})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(render.RenderBootstrapToken(t))
		},
	}
	tokensCreateCmd.Flags().StringVar(&ttl, "ttl", "300s", "Time to live")
	tokensCreateCmd.Flags().StringSliceVar(&labels, "labels", []string{}, "Labels which will be auto-applied to any clusters created with this token")
	tokensCreateCmd.Flags().IntVar(&maxUsages, "max-usages", 0, "Maximum number of times this token can be used, the default is 0 which is unlimited")
	return tokensCreateCmd
}

func BuildTokensCreateSupportCmd() *cobra.Command {
	var ttl string
	var username string
	tokensCreateSupportCmd := &cobra.Command{
		Use:   "create-support",
		Short: "Create a bootstrap token for the support agent",
		Run: func(cmd *cobra.Command, args []string) {
			duration, err := time.ParseDuration(ttl)
			if err != nil {
				lg.Fatal(err)
			}

			labels := map[string]string{
				corev1.NameLabel:    username,
				corev1.SupportLabel: "true",
			}
			t, err := mgmtClient.CreateBootstrapToken(cmd.Context(),
				&managementv1.CreateBootstrapTokenRequest{
					Ttl:       durationpb.New(duration),
					Labels:    labels,
					MaxUsages: 1,
				})
			if err != nil {
				lg.Fatal(err)
			}
			fmt.Println(render.RenderBootstrapToken(t))
		},
	}
	tokensCreateSupportCmd.Flags().StringVar(&ttl, "ttl", "168h", "Time to live")
	tokensCreateSupportCmd.Flags().StringVar(&username, "username", "", "Username of the support agent")
	return tokensCreateSupportCmd
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
			fmt.Println(render.RenderBootstrapTokenList(t))
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
			render.RenderBootstrapTokenList(&corev1.BootstrapTokenList{
				Items: tokenList,
			})
		},
	}
}

func init() {
	AddCommandsToGroup(ManagementAPI, BuildTokensCmd())
}
