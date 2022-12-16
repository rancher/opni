package commands

import (
	"fmt"
	cliutil "github.com/rancher/opni/pkg/opni/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"
	"strings"
	"time"
)

// todo: add cluster id as a positional arg where appropriate

func parseLabelMatcher(s string) (*remoteread.LabelMatcher, error) {
	if strings.Contains(s, "!~") {
		split := strings.SplitN(s, "!~", 2)

		return &remoteread.LabelMatcher{
			Type:  remoteread.LabelMatcher_NOT_REGEX_EQUAL,
			Name:  split[0],
			Value: split[1],
		}, nil
	} else if strings.Contains(s, "=~") {
		split := strings.SplitN(s, "=~", 2)

		return &remoteread.LabelMatcher{
			Type:  remoteread.LabelMatcher_REGEX_EQUAL,
			Name:  split[0],
			Value: split[1],
		}, nil
	} else if strings.Contains(s, "!=") {
		split := strings.SplitN(s, "!=", 2)

		return &remoteread.LabelMatcher{
			Type:  remoteread.LabelMatcher_NOT_EQUAL,
			Name:  split[0],
			Value: split[1],
		}, nil
	} else if strings.Contains(s, "=") {
		split := strings.SplitN(s, "=", 2)

		return &remoteread.LabelMatcher{
			Type:  remoteread.LabelMatcher_EQUAL,
			Name:  split[0],
			Value: split[1],
		}, nil
	}

	return &remoteread.LabelMatcher{}, fmt.Errorf("label matcher must contain one of =, !=, =~, or !~")
}

func BuildImportTargetAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <endpoint> [name]",
		Short: "Add a new target",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var target *remoteread.Target

			switch len(args) {
			case 1:
				target = &remoteread.Target{
					Name:     args[0],
					Endpoint: args[0],
				}
			case 2:
				// todo: might be worthwhile pulling just the hostname from the given endpoint
				target = &remoteread.Target{
					Name:     args[1],
					Endpoint: args[0],
				}
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

func BuildImportTargetEditCmd() *cobra.Command {
	var newEndpoint string
	var newName string

	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit an existing target",
		RunE: func(cmd *cobra.Command, args []string) error {
			if newEndpoint == "" && newName == "" {
				lg.Infof("no edits specified, doing nothing")
			}

			request := &remoteread.TargetEditRequest{TargetDiff: &remoteread.TargetDiff{
				Endpoint: newEndpoint,
				Name:     newName,
			}}

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

func BuildImportTargetRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <name>",
		Short:   "Remove a target",
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			request := &remoteread.TargetRemoveRequest{TargetName: args[0]}

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

func BuildImportTargetListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List available targets",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			request := &remoteread.TargetListRequest{ClusterId: ""}

			targetList, err := remoteReadClient.ListTargets(cmd.Context(), request)
			if err != nil {
				return err
			}

			fmt.Println(cliutil.RenderTargetList(targetList))

			return nil
		},
	}

	return cmd
}

func BuildImportTargetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "target",
		Short:   "target",
		Aliases: []string{"targets"},
	}

	// todo: scan && describe
	cmd.AddCommand(BuildImportTargetAddCmd())
	cmd.AddCommand(BuildImportTargetEditCmd())
	cmd.AddCommand(BuildImportTargetRemoveCmd())
	cmd.AddCommand(BuildImportTargetListCmd())

	return cmd
}

func BuildImportStartCmd() *cobra.Command {
	var labelFilters []string
	var startTimestamp int64
	var endTimestamp int64
	var clusterId string
	var forceOverlap bool

	cmd := &cobra.Command{
		Use:   "start <target>",
		Short: "start",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetName := args[0]
			labelMatchers := make([]*remoteread.LabelMatcher, len(labelFilters))

			for _, labelFilter := range labelFilters {
				matcher, err := parseLabelMatcher(labelFilter)

				if err != nil {
					return fmt.Errorf("filter '%s' is not valid: %w", labelFilter, err)
				}

				labelMatchers = append(labelMatchers, matcher)
			}

			query := &remoteread.Query{
				StartTimestamp: &timestamppb.Timestamp{Seconds: startTimestamp},
				EndTimestamp:   &timestamppb.Timestamp{Seconds: endTimestamp},
				Matchers:       labelMatchers,
			}

			request := &remoteread.StartReadRequest{
				TargetName:   targetName,
				Query:        query,
				ForceOverlap: forceOverlap,
			}

			if _, err := remoteReadClient.Start(cmd.Context(), request); err != nil {
				return err
			}

			lg.Infof("import started")

			return nil
		},
	}

	cmd.Flags().StringVar(&clusterId, "cluster", "", "the id of the cluster")

	cmd.Flags().StringSliceVar(&labelFilters, "filters", []string{"__name__=~\".+\""}, "promql query for the thing")

	// todo: we probably want to allow for more human readable timestamps here
	cmd.Flags().Int64Var(&startTimestamp, "start", 0, "start time for the remote read")
	cmd.Flags().Int64Var(&endTimestamp, "end", time.Now().Unix(), "start time for the remote read")

	cmd.Flags().BoolVar(&forceOverlap, "force", false, "force import when 'start' is before the last stored start")

	ConfigureManagementCommand(cmd)
	ConfigureImportCommand(cmd)

	return cmd
}

func BuildImportStopCmd() *cobra.Command {
	var clusterId string

	cmd := &cobra.Command{
		Use:   "start <target>",
		Short: "start",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetName := args[0]

			request := &remoteread.StopReadRequest{
				TargetName: targetName,
				ClusterId:  clusterId,
			}

			if _, err := remoteReadClient.Stop(cmd.Context(), request); err != nil {
				return err
			}

			lg.Infof("import stopped")

			return nil
		},
	}

	cmd.Flags().StringVar(&clusterId, "cluster", "", "the id of the cluster")

	ConfigureManagementCommand(cmd)
	ConfigureImportCommand(cmd)

	return cmd
}

func BuildImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Interact with metrics import plugin APIs",
	}

	cmd.AddCommand(BuildImportTargetCmd())
	cmd.AddCommand(BuildImportStartCmd())
	cmd.AddCommand(BuildImportStopCmd())

	ConfigureManagementCommand(cmd)
	ConfigureImportCommand(cmd)

	return cmd
}

func init() {
	AddCommandsToGroup(PluginAPIs, BuildImportCmd())
}
