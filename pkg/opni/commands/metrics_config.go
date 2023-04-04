package commands

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/google/go-cmp/cmp"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	cliutil "github.com/rancher/opni/pkg/opni/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/node"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/emptypb"
)

func BuildMetricsConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "View and configure the metrics capability",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			defaultConfig, err := nodeConfigClient.GetDefaultConfiguration(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}
			allAgents, err := mgmtClient.ListClusters(cmd.Context(), &managementv1.ListClustersRequest{})
			if err != nil {
				return err
			}

			items := make([]cliutil.MetricsNodeConfigInfo, len(allAgents.Items))
			for i, agent := range allAgents.Items {
				var trailer metadata.MD
				spec, err := nodeConfigClient.GetNodeConfiguration(cmd.Context(), agent.Reference(), grpc.Trailer(&trailer))
				if err != nil {
					return err
				}
				var isDefault bool
				if vv := trailer.Get("is-default-config"); len(vv) == 1 && vv[0] == "true" {
					isDefault = true
				}

				items[i] = cliutil.MetricsNodeConfigInfo{
					Id:            agent.Id,
					HasCapability: capabilities.Has(agent, capabilities.Cluster(wellknown.CapabilityMetrics)),
					Spec:          spec,
					IsDefault:     isDefault,
				}
			}

			fmt.Println(cliutil.RenderMetricsNodeConfigs(items, defaultConfig))
			return nil
		},
	}
	cmd.AddCommand(BuildMetricsConfigSetCmd())
	cmd.AddCommand(BuildMetricsConfigGetCmd())
	cmd.AddCommand(BuildMetricsConfigResetCmd())
	cmd.AddCommand(BuildMetricsConfigSetDefaultCmd())
	cmd.AddCommand(BuildMetricsConfigGetDefaultCmd())
	cmd.AddCommand(BuildMetricsConfigResetDefaultCmd())
	return cmd
}

func BuildMetricsConfigSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "set <cluster-id> [config]",
		Aliases: []string{"edit"},
		Short:   "Set metrics configuration for a cluster",
		Args:    cobra.RangeArgs(1, 2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return completeClusters(cmd, args, toComplete)
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterID := args[0]
			var spec *node.MetricsCapabilitySpec
			var err error
			if len(args) == 2 {
				spec, err = parseSpec(args[1])
				if err != nil {
					return err
				}
			} else {
				// fetch the current spec, open it in an editor (json), and then
				// parse the result
				conf, err := nodeConfigClient.GetNodeConfiguration(cmd.Context(), &corev1.Reference{
					Id: clusterID,
				})
				if err != nil {
					return err
				}
				updated, err := editSpec(conf)
				if err != nil {
					return err
				}
				if cmp.Equal(conf, updated, protocmp.Transform()) {
					fmt.Println("No changes made")
					return nil
				}
				spec = updated
			}
			_, err = nodeConfigClient.SetNodeConfiguration(cmd.Context(), &node.NodeConfigRequest{
				Node: &corev1.Reference{
					Id: clusterID,
				},
				Spec: spec,
			})
			if err != nil {
				return err
			}
			fmt.Printf("Metrics configuration set for cluster %q\n", clusterID)
			return nil
		},
	}
	return cmd
}

func BuildMetricsConfigGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <cluster-id> [cluster-id ...]",
		Short: "Get metrics configuration for one or more clusters",
		Args:  cobra.MinimumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeClusters(cmd, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			infos := make([]cliutil.MetricsNodeConfigInfo, 0, len(args))
			for _, clusterID := range args {
				ref := &corev1.Reference{Id: clusterID}
				var trailer metadata.MD
				spec, err := nodeConfigClient.GetNodeConfiguration(cmd.Context(), ref, grpc.Trailer(&trailer))
				if err != nil {
					return err
				}
				cluster, err := mgmtClient.GetCluster(cmd.Context(), ref)
				if err != nil {
					return err
				}

				var isDefault bool
				if vv := trailer.Get("is-default-config"); len(vv) == 1 && vv[0] == "true" {
					isDefault = true
				}

				infos = append(infos, cliutil.MetricsNodeConfigInfo{
					Id:            clusterID,
					HasCapability: capabilities.Has(cluster, capabilities.Cluster(wellknown.CapabilityMetrics)),
					Spec:          spec,
					IsDefault:     isDefault,
				})
			}
			fmt.Println(cliutil.RenderMetricsNodeConfigs(infos, nil))
			return nil
		},
	}
	return cmd
}

func BuildMetricsConfigResetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset <cluster-id> [cluster-id ...]",
		Args:  cobra.MinimumNArgs(1),
		Short: "Reset the metrics capability configuration for a cluster to the default",
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeClusters(cmd, args, toComplete, func(c *corev1.Cluster) bool {
				// filter for clusters that are using the default config
				var trailer metadata.MD
				client := node.NewNodeConfigurationClient(managementv1.UnderlyingConn(mgmtClient))
				_, err := client.GetNodeConfiguration(cmd.Context(), c.Reference(), grpc.Trailer(&trailer))
				if err == nil {
					if len(trailer.Get("is-default-config")) == 0 {
						return true
					}
					return false
				}
				// on error, show it anyway
				return true
			})
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, clusterID := range args {
				// set the config with a nil spec to reset it
				_, err := nodeConfigClient.SetNodeConfiguration(cmd.Context(), &node.NodeConfigRequest{
					Node: &corev1.Reference{
						Id: clusterID,
					},
					Spec: nil,
				})
				if err != nil {
					fmt.Printf("Failed to reset metrics capability configuration for cluster %q: %v\n", clusterID, err)
				}
			}
			return nil
		},
	}
	return cmd
}

func BuildMetricsConfigSetDefaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "set-default [config]",
		Aliases: []string{"edit-default"},
		Short:   "Set the default metrics configuration (applies to all clusters without a specific configuration)",
		Args:    cobra.RangeArgs(0, 1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var spec *node.MetricsCapabilitySpec
			var err error
			if len(args) == 1 {
				spec, err = parseSpec(args[0])
				if err != nil {
					return err
				}
			} else {
				// fetch the current spec, open it in an editor (json), and then
				// parse the result
				spec, err = nodeConfigClient.GetDefaultConfiguration(cmd.Context(), &emptypb.Empty{})
				if err != nil {
					return err
				}
				updated, err := editSpec(spec)
				if err != nil {
					return err
				}
				if cmp.Equal(spec, updated, protocmp.Transform()) {
					fmt.Println("No changes made")
					return nil
				}
				spec = updated
			}

			_, err = nodeConfigClient.SetDefaultConfiguration(cmd.Context(), spec)
			if err != nil {
				return err
			}
			fmt.Println("Default metrics configuration set")
			return nil
		},
	}
	return cmd
}

func BuildMetricsConfigGetDefaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-default",
		Short: "Get the default metrics configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := nodeConfigClient.GetDefaultConfiguration(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return err
			}

			fmt.Println(cliutil.RenderDefaultNodeConfig(spec))
			return nil
		},
	}
	return cmd
}

func BuildMetricsConfigResetDefaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset-default",
		Short: "Reset the default metrics configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := nodeConfigClient.SetDefaultConfiguration(cmd.Context(), &node.MetricsCapabilitySpec{})
			if err != nil {
				return err
			}
			fmt.Println("Default metrics configuration reset")
			return nil
		},
	}
	return cmd
}

func parseSpec(input string) (*node.MetricsCapabilitySpec, error) {
	spec := &node.MetricsCapabilitySpec{}
	if err := protojson.Unmarshal([]byte(input), spec); err != nil {
		return nil, err
	}
	return spec, nil
}

var errAborted = errors.New("aborted by user")

func editSpec(spec *node.MetricsCapabilitySpec, id ...string) (*node.MetricsCapabilitySpec, error) {
	var err error
	for {
		var extraComments []string
		if len(id) > 0 {
			extraComments = []string{fmt.Sprintf("[id: %s]", id[0])}
		}
		if err != nil {
			extraComments = []string{fmt.Sprintf("error: %v", err)}
		}
		var editedSpec *node.MetricsCapabilitySpec
		editedSpec, err = tryEditSpec(spec, extraComments)
		if err != nil {
			if errors.Is(err, errAborted) {
				return nil, err
			}
			continue
		}
		return editedSpec, nil
	}
}

func tryEditSpec(spec *node.MetricsCapabilitySpec, extraComments []string) (*node.MetricsCapabilitySpec, error) {
	jsonData, err := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: true,
	}.Marshal(spec)
	if err != nil {
		return nil, err
	}

	for i, comment := range extraComments {
		if !strings.HasPrefix(comment, "//") {
			extraComments[i] = "// " + comment
		}
	}

	// Add comments to the JSON
	comments := append([]string{
		"// Edit the metrics capability config below. Comments are ignored.",
		"// If everything is deleted, the operation will be aborted.",
	}, extraComments...)
	specWithComments := strings.Join(append(comments, string(jsonData)), "\n")

	// Create a temporary file for editing
	tmpFile, err := os.CreateTemp("", "metrics-config-*.json")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())

	// Write the JSON with comments to the temporary file
	if _, err := tmpFile.WriteString(specWithComments); err != nil {
		return nil, err
	}
	if err := tmpFile.Close(); err != nil {
		return nil, err
	}

	// Open the temporary file in the user's preferred editor
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim" // Default to 'vim' if no editor is set
	}

	args := []string{tmpFile.Name()}
	if editor != "vi" {
		// set jsonc syntax highlighting for editors other than vi
		args = append(args, "+set ft=jsonc")
	}

	cmd := exec.Command(editor, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("editor command failed: %w", err)
	}

	// Read the edited JSON
	editedBytes, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return nil, err
	}

	editedBytes = bytes.TrimSpace(editedBytes)

	// Remove comments and empty lines
	editedLines := strings.Split(string(editedBytes), "\n")
	filteredLines := make([]string, 0, len(editedLines))
	for _, line := range editedLines {
		if !strings.HasPrefix(strings.TrimSpace(line), "//") {
			filteredLines = append(filteredLines, line)
		}
	}

	// If everything is deleted, abort the operation
	if len(filteredLines) == 0 {
		return nil, errAborted
	}

	// Unmarshal the edited JSON into a new spec
	editedSpec := &node.MetricsCapabilitySpec{}
	if err := protojson.Unmarshal([]byte(strings.Join(filteredLines, "\n")), editedSpec); err != nil {
		return nil, err
	}

	return editedSpec, nil
}
