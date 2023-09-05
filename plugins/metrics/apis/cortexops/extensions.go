package cortexops

import (
	context "context"
	"fmt"
	"os"
	"time"

	"github.com/rancher/opni/internal/codegen/cli"
	cliutil "github.com/rancher/opni/pkg/opni/cliutil"
	driverutil "github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/tui"
	cobra "github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"golang.org/x/term"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

func (s *InstallStatus) RenderText(out cli.Writer) {
	out.Print("Configuration State: ")
	switch s.ConfigState {
	case ConfigurationState_NotConfigured:
		out.Println(chalk.Dim.TextStyle("Not Configured"))
	case ConfigurationState_Configured:
		out.Println(chalk.Green.Color("Configured"))
	}

	out.Print("Install State: ")
	switch s.InstallState {
	case InstallState_NotInstalled:
		out.Println(chalk.Dim.TextStyle("Not Installed"))
	case InstallState_Installed:
		out.Println(chalk.Green.Color("Installed"))
	}

	out.Print("Application State: ")
	switch s.AppState {
	case ApplicationState_NotRunning:
		out.Println(chalk.Dim.TextStyle("Not Running"))
	case ApplicationState_Pending:
		out.Println(chalk.Yellow.Color("Pending"))
	case ApplicationState_Running:
		out.Println(chalk.Green.Color("Running"))
	case ApplicationState_Failed:
		out.Println(chalk.Red.Color("Failed"))
	}
	if len(s.GetWarnings()) > 0 {
		out.Println(chalk.Yellow.Color("Warnings:"))
		for _, warning := range s.GetWarnings() {
			out.Printf(" - %s\n", warning)
		}
	}

	out.Printf("Version: %s\n", s.Version)
	for k, v := range s.Metadata {
		out.Printf("%s: %s\n", k, v)
	}
}

func (h *ConfigurationHistoryResponse) RenderText(out cli.Writer) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		out.Println(driverutil.MarshalConfigJson(h))
		return
	}
	ui := tui.NewHistoryUI(h.GetEntries())
	ui.Run()
}

func init() {
	addBuildHook_CortexOpsSetConfiguration(func(c *cobra.Command) {
		c.Flags().String("from-preset", "", "optional preset to use. If specified, all other flags are ignored")
		c.RegisterFlagCompletionFunc("from-preset", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			cliutil.BasePreRunE(cmd, args)
			client, ok := CortexOpsClientFromContext(cmd.Context())
			if !ok {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}

			presets, err := client.ListPresets(cmd.Context(), &emptypb.Empty{})
			if err != nil {
				return nil, cobra.ShellCompDirectiveError
			}

			names := make([]string, len(presets.Items))
			for i, preset := range presets.Items {
				names[i] = preset.GetId().GetId()
			}
			return names, cobra.ShellCompDirectiveNoFileComp
		})
		defaultRunE := c.RunE
		c.RunE = func(cmd *cobra.Command, args []string) error {
			if fromPreset := cmd.Flags().Lookup("from-preset").Value.String(); fromPreset != "" {
				client, ok := CortexOpsClientFromContext(cmd.Context())
				if !ok {
					return nil
				}

				presets, err := client.ListPresets(cmd.Context(), &emptypb.Empty{})
				if err != nil {
					return err
				}

				for _, preset := range presets.Items {
					if preset.GetId().GetId() == fromPreset {
						_, err := client.SetConfiguration(cmd.Context(), preset.GetSpec())
						return err
					}
				}
			}

			return defaultRunE(cmd, args)
		}
	})

	addBuildHook_CortexOpsGetConfiguration(func(c *cobra.Command) {
		c.RegisterFlagCompletionFunc("revision", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			cliutil.BasePreRunE(cmd, args)
			return completeRevisions(cmd.Context(), driverutil.Target_ActiveConfiguration)
		})
	})

	addBuildHook_CortexOpsGetDefaultConfiguration(func(c *cobra.Command) {
		c.RegisterFlagCompletionFunc("revision", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			cliutil.BasePreRunE(cmd, args)
			return completeRevisions(cmd.Context(), driverutil.Target_DefaultConfiguration)
		})
	})

	// NB: order matters here in order to pick up the build hook correctly
	// this is not added in dryrun.go because it is in the wrong order
	// alphabetically by filename
	addExtraCortexOpsCmd(BuildDryRunCmd())
	addExtraCortexOpsCmd(BuildRollbackCmd())
}

func completeRevisions(ctx context.Context, target driverutil.Target) ([]string, cobra.ShellCompDirective) {
	client, ok := CortexOpsClientFromContext(ctx)
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	history, err := client.ConfigurationHistory(ctx, &ConfigurationHistoryRequest{
		Target:        target,
		IncludeValues: false,
	})
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	revisions := make([]string, len(history.GetEntries()))
	for i, entry := range history.GetEntries() {
		comp := fmt.Sprint(entry.GetRevision().GetRevision())
		ts := entry.GetRevision().GetTimestamp().AsTime()
		if !ts.IsZero() {
			comp = fmt.Sprintf("%s\t%s", comp, ts.Format(time.Stamp))
		}
		revisions[i] = comp
	}
	return revisions, cobra.ShellCompDirectiveNoFileComp
}
