package commands

import (
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/emptypb"
)

var CompletionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Generate completion script",
	Long: `To load completions:

Bash:

  $ source <(opni completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ opni completion bash > /etc/bash_completion.d/opni
  # macOS:
  $ opni completion bash > /usr/local/etc/bash_completion.d/opni

Zsh:

  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ opni completion zsh > "${fpath[1]}/_opni"

  # You will need to start a new shell for this setup to take effect.

fish:

  $ opni completion fish | source

  # To load completions for each session, execute once:
  $ opni completion fish > ~/.config/fish/completions/opni.fish
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish"},
	Args:                  cobra.ExactValidArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			cmd.Root().GenFishCompletion(os.Stdout, true)
		}
	},
}

func completeCapability(cmd *cobra.Command, _ []string, toComplete string, filters ...func(*managementv1.CapabilityInfo) bool) ([]string, cobra.ShellCompDirective) {
	if err := managementPreRunE(cmd, nil); err != nil {
		return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
	}
	capabilities, err := mgmtClient.ListCapabilities(cmd.Context(), &emptypb.Empty{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	for _, f := range filters {
		capabilities.Items = lo.Filter(capabilities.Items, util.Indexed(f))
	}

	return lo.Filter(capabilities.Names(), func(name string, _ int) bool {
		return strings.HasPrefix(name, toComplete)
	}), cobra.ShellCompDirectiveNoFileComp
}

func filterNodeCountNonZero(info *managementv1.CapabilityInfo) bool {
	return info.NodeCount > 0
}

func completeClusters(cmd *cobra.Command, args []string, toComplete string, filters ...func(*corev1.Cluster) bool) ([]string, cobra.ShellCompDirective) {
	if err := managementPreRunE(cmd, nil); err != nil {
		return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
	}
	clusters, err := mgmtClient.ListClusters(cmd.Context(), &managementv1.ListClustersRequest{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	for _, f := range filters {
		clusters.Items = lo.Filter(clusters.Items, util.Indexed(f))
	}

	var comps []string
	for _, cluster := range clusters.Items {
		name := cluster.GetLabels()[corev1.NameLabel]
		id := cluster.Id

		if slices.Contains(args, id) || slices.Contains(args, name) {
			continue
		}

		if strings.HasPrefix(id, toComplete) {
			if name != "" {
				comps = append(comps, fmt.Sprintf("%s\t%s", id, name))
			} else {
				comps = append(comps, id)
			}
		} else if strings.HasPrefix(name, toComplete) {
			comps = append(comps, fmt.Sprintf("%s\t%s", name, id))
		}
	}
	return comps, cobra.ShellCompDirectiveNoFileComp
}

func filterHasCapability(capability string) func(*corev1.Cluster) bool {
	return func(c *corev1.Cluster) bool {
		return capabilities.Has(c, capabilities.Cluster(capability))
	}
}

func filterDoesNotHaveCapability(capability string) func(*corev1.Cluster) bool {
	return func(c *corev1.Cluster) bool {
		return !capabilities.Has(c, capabilities.Cluster(capability))
	}
}

func completeRoleBindings(cmd *cobra.Command, args []string, toComplete string, filters ...func(*corev1.RoleBinding) bool) ([]string, cobra.ShellCompDirective) {
	if err := managementPreRunE(cmd, nil); err != nil {
		return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
	}
	roleBindings, err := mgmtClient.ListRoleBindings(cmd.Context(), &emptypb.Empty{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	for _, f := range filters {
		roleBindings.Items = lo.Filter(roleBindings.Items, util.Indexed(f))
	}

	var comps []string
	for _, roleBinding := range roleBindings.Items {
		if slices.Contains(args, roleBinding.Id) {
			continue
		}
		if strings.HasPrefix(roleBinding.Id, toComplete) {
			comps = append(comps, fmt.Sprintf("%s\t%s", roleBinding.Id, fmt.Sprintf("role: %s, subjects: %v", roleBinding.RoleId, roleBinding.Subjects)))
		}
	}
	return comps, cobra.ShellCompDirectiveNoFileComp
}

func completeRoles(cmd *cobra.Command, args []string, toComplete string, filters ...func(*corev1.Role) bool) ([]string, cobra.ShellCompDirective) {
	if err := managementPreRunE(cmd, nil); err != nil {
		return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
	}
	roles, err := mgmtClient.ListRoles(cmd.Context(), &emptypb.Empty{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	for _, f := range filters {
		roles.Items = lo.Filter(roles.Items, util.Indexed(f))
	}

	var comps []string
	for _, role := range roles.Items {
		if slices.Contains(args, role.Id) {
			continue
		}
		if strings.HasPrefix(role.Id, toComplete) {
			comps = append(comps, fmt.Sprintf("%s\t%s", role.Id, role.MatchLabels.ExpressionString()))
		}
	}
	return comps, cobra.ShellCompDirectiveNoFileComp
}

func completeBootstrapTokens(cmd *cobra.Command, args []string, toComplete string, filters ...func(*corev1.BootstrapToken) bool) ([]string, cobra.ShellCompDirective) {
	if err := managementPreRunE(cmd, nil); err != nil {
		return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
	}
	tokens, err := mgmtClient.ListBootstrapTokens(cmd.Context(), &emptypb.Empty{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	for _, f := range filters {
		tokens.Items = lo.Filter(tokens.Items, util.Indexed(f))
	}

	var comps []string
	for _, token := range tokens.Items {
		if slices.Contains(args, token.TokenID) {
			continue
		}
		if strings.HasPrefix(token.TokenID, toComplete) {
			desc := fmt.Sprintf("ttl: %s", (time.Duration(token.GetMetadata().GetTtl()) * time.Second).String())
			if token.GetMetadata().GetUsageCount() > 0 {
				desc = fmt.Sprintf("%s, usages: %d", desc, token.GetMetadata().GetUsageCount())
			}
			if len(token.GetMetadata().GetLabels()) > 0 {
				desc = fmt.Sprintf("%s, labels: %v", desc, token.GetMetadata().GetLabels())
			}
			comps = append(comps, fmt.Sprintf("%s\t%s", token.TokenID, desc))
		}
	}
	return comps, cobra.ShellCompDirectiveNoFileComp
}
