package commands

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	cliutil "github.com/rancher/opni/pkg/opni/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"
	"strings"
	"time"
)

func parseLabelMatcher(s string) (*remoteread.LabelMatcher, error) {
	if strings.Contains(s, "!~") {
		split := strings.SplitN(s, "!~", 2)

		return &remoteread.LabelMatcher{
			Type:  remoteread.LabelMatcher_NotRegexEqual,
			Name:  split[0],
			Value: split[1],
		}, nil
	} else if strings.Contains(s, "=~") {
		split := strings.SplitN(s, "=~", 2)

		return &remoteread.LabelMatcher{
			Type:  remoteread.LabelMatcher_RegexEqual,
			Name:  split[0],
			Value: split[1],
		}, nil
	} else if strings.Contains(s, "!=") {
		split := strings.SplitN(s, "!=", 2)

		return &remoteread.LabelMatcher{
			Type:  remoteread.LabelMatcher_NotEqual,
			Name:  split[0],
			Value: split[1],
		}, nil
	} else if strings.Contains(s, "=") {
		split := strings.SplitN(s, "=", 2)

		return &remoteread.LabelMatcher{
			Type:  remoteread.LabelMatcher_Equal,
			Name:  split[0],
			Value: split[1],
		}, nil
	}

	return &remoteread.LabelMatcher{}, fmt.Errorf("label matcher must contain one of =, !=, =~, or !~")
}

func followProgress(ctx context.Context, name string, cluster string) error {
	request := &remoteread.TargetStatusRequest{
		Meta: &remoteread.TargetMeta{
			Name:      name,
			ClusterId: cluster,
		},
	}

	model := NewProgressModel(ctx, request)

	if err := tea.NewProgram(model).Start(); err != nil {
		return fmt.Errorf("could not render progress: %w", err)
	}

	return nil
}

func BuildImportAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <cluster> <name> <endpoint>",
		Short: "Add a new import target",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterId := args[0]
			targetName := args[1]
			endpoint := args[2]

			target := &remoteread.Target{
				Meta: &remoteread.TargetMeta{
					ClusterId: clusterId,
					Name:      targetName,
				},
				Spec: &remoteread.TargetSpec{
					Endpoint: endpoint,
				},
			}

			request := &remoteread.TargetAddRequest{
				Target: target,
			}

			_, err := remoteReadClient.AddTarget(cmd.Context(), request)

			if err != nil {
				return err
			}

			lg.Infof("target added")
			return nil
		},
	}

	return cmd
}

func BuildImportEditCmd() *cobra.Command {
	var newEndpoint string
	var newName string

	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit an existing import target",
		RunE: func(cmd *cobra.Command, args []string) error {
			if newEndpoint == "" && newName == "" {
				lg.Infof("no edits specified, doing nothing")
			}

			request := &remoteread.TargetEditRequest{
				Meta: &remoteread.TargetMeta{
					ClusterId: args[0],
					Name:      args[1],
				},
				TargetDiff: &remoteread.TargetDiff{
					Endpoint: newEndpoint,
					Name:     newName,
				},
			}

			_, err := remoteReadClient.EditTarget(cmd.Context(), request)

			if err != nil {
				return err
			}

			lg.Infof("target edited")
			return nil
		},
	}

	cmd.Flags().StringVar(&newEndpoint, "endpoint", "", "the new endpoint for the target")

	cmd.Flags().StringVar(&newName, "name", "", "the new name for the target")

	return cmd
}

func BuildImportRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <cluster> <name>",
		Short:   "Remove an import target",
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			request := &remoteread.TargetRemoveRequest{
				Meta: &remoteread.TargetMeta{
					ClusterId: args[0],
					Name:      args[1],
				},
			}

			_, err := remoteReadClient.RemoveTarget(cmd.Context(), request)

			if err != nil {
				return err
			}

			lg.Infof("target removed")
			return nil
		},
	}

	return cmd
}

func BuildImportListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List available import targets",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			request := &remoteread.TargetListRequest{ClusterId: ""}

			targetList, err := remoteReadClient.ListTargets(cmd.Context(), request)
			if err != nil {
				return err
			}

			if targetList == nil {
				return fmt.Errorf("received list is nil")
			}

			fmt.Println(cliutil.RenderTargetList(targetList))

			return nil
		},
	}

	return cmd
}

func BuildImportStartCmd() *cobra.Command {
	var labelFilters []string
	var startTimestampSecs int64
	var endTimestampSecs int64
	var forceOverlap bool
	var follow bool

	cmd := &cobra.Command{
		Use:   "start <cluster> <target>",
		Short: "start an import",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterId := args[0]
			targetName := args[1]
			labelMatchers := make([]*remoteread.LabelMatcher, len(labelFilters))

			for _, labelFilter := range labelFilters {
				matcher, err := parseLabelMatcher(labelFilter)

				if err != nil {
					return fmt.Errorf("filter '%s' is not valid: %w", labelFilter, err)
				}

				labelMatchers = append(labelMatchers, matcher)
			}

			query := &remoteread.Query{
				StartTimestamp: &timestamppb.Timestamp{Seconds: startTimestampSecs},
				EndTimestamp:   &timestamppb.Timestamp{Seconds: endTimestampSecs},
				Matchers:       labelMatchers,
			}

			request := &remoteread.StartReadRequest{
				Target: &remoteread.Target{
					Meta: &remoteread.TargetMeta{
						ClusterId: clusterId,
						Name:      targetName,
					},
				},
				Query:        query,
				ForceOverlap: forceOverlap,
			}

			if _, err := remoteReadClient.Start(cmd.Context(), request); err != nil {
				return err
			}

			lg.Infof("import started")

			if follow {
				return followProgress(cmd.Context(), targetName, clusterId)
			}

			return nil
		},
	}

	// todo: this default does not pull anything, but job=prometheus-poc did
	cmd.Flags().StringSliceVar(&labelFilters, "filters", []string{"__name__=~\".+\""}, "label matchers to use for the import")

	// todo: we probably want to allow for more human-readable timestamps here
	cmd.Flags().Int64Var(&startTimestampSecs, "start", time.Now().Unix()-int64(time.Hour.Seconds())*3, "start time for the remote read in seconds since epoch")
	cmd.Flags().Int64Var(&endTimestampSecs, "end", time.Now().Unix(), "start time for the remote read")

	cmd.Flags().BoolVar(&follow, "follow", false, "follow import progress (the same as calling start then progress immediately)")

	cmd.Flags().BoolVar(&forceOverlap, "force", false, "force import when 'start' is before the last stored start")

	ConfigureManagementCommand(cmd)
	ConfigureImportCommand(cmd)

	return cmd
}

func BuildImportStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <cluster> <target>",
		Short: "stop an import (will not remove already imported data)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterId := args[0]
			targetName := args[1]

			request := &remoteread.StopReadRequest{
				Meta: &remoteread.TargetMeta{
					ClusterId: clusterId,
					Name:      targetName,
				},
			}

			if _, err := remoteReadClient.Stop(cmd.Context(), request); err != nil {
				return err
			}

			lg.Infof("import stopped")

			return nil
		},
	}

	ConfigureManagementCommand(cmd)
	ConfigureImportCommand(cmd)

	return cmd
}

func BuildProgressCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "progress <cluster> <target>",
		Short: "follow import progress until finished",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusterId := args[0]
			targetName := args[1]

			return followProgress(cmd.Context(), targetName, clusterId)
		},
	}

	ConfigureManagementCommand(cmd)
	ConfigureImportCommand(cmd)

	return cmd
}

func BuildImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Interact with metrics import plugin APIs",
	}

	cmd.AddCommand(BuildImportAddCmd())
	cmd.AddCommand(BuildImportEditCmd())
	cmd.AddCommand(BuildImportRemoveCmd())
	cmd.AddCommand(BuildImportListCmd())
	cmd.AddCommand(BuildImportStartCmd())
	cmd.AddCommand(BuildImportStopCmd())
	cmd.AddCommand(BuildProgressCmd())

	ConfigureManagementCommand(cmd)
	ConfigureImportCommand(cmd)

	return cmd
}

func init() {
	AddCommandsToGroup(PluginAPIs, BuildImportCmd())
}
