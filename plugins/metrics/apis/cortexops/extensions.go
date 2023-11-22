package cortexops

import (
	"os"

	"github.com/bufbuild/protovalidate-go"
	"github.com/rancher/opni/internal/codegen/cli"
	cliutil "github.com/rancher/opni/pkg/opni/cliutil"
	driverutil "github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/driverutil/complete"
	"github.com/rancher/opni/pkg/plugins/driverutil/dryrun"
	"github.com/rancher/opni/pkg/plugins/driverutil/rollback"
	"github.com/rancher/opni/pkg/tui"
	cobra "github.com/spf13/cobra"
	"golang.org/x/term"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

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
			client, ok := CortexOpsContextInjector.ClientFromContext(cmd.Context())
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
				client, ok := CortexOpsContextInjector.ClientFromContext(cmd.Context())
				if !ok {
					return nil
				}

				presets, err := client.ListPresets(cmd.Context(), &emptypb.Empty{})
				if err != nil {
					return err
				}

				for _, preset := range presets.Items {
					if preset.GetId().GetId() == fromPreset {
						spec := preset.GetSpec()
						if cmd.Flags().Lookup("interactive").Value.String() == "true" {
							drr, err := client.DryRun(cmd.Context(), &DryRunRequest{
								Target: driverutil.Target_ActiveConfiguration,
								Action: driverutil.Action_Set,
								Spec:   spec,
							})
							if err != nil {
								return err
							}
							modified := drr.GetModified()
							comments := []string{}
							if errs := (*protovalidate.ValidationError)(drr.GetValidationErrors()); errs != nil {
								comments = append(comments, errs.Error())
							}
							if modified, err = cliutil.EditInteractive(modified, comments...); err != nil {
								return err
							}
							spec = modified
						}
						_, err := client.SetConfiguration(cmd.Context(), &SetRequest{
							Spec: spec,
						})
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
			client, ok := CortexOpsContextInjector.ClientFromContext(cmd.Context())
			if !ok {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return complete.Revisions(cmd.Context(), &driverutil.ConfigurationHistoryRequest{
				Target: driverutil.Target_ActiveConfiguration,
			}, client)
		})
	})

	addBuildHook_CortexOpsGetDefaultConfiguration(func(c *cobra.Command) {
		c.RegisterFlagCompletionFunc("revision", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			cliutil.BasePreRunE(cmd, args)
			client, ok := CortexOpsContextInjector.ClientFromContext(cmd.Context())
			if !ok {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return complete.Revisions(cmd.Context(), &driverutil.ConfigurationHistoryRequest{
				Target: driverutil.Target_DefaultConfiguration,
			}, client)
		})
	})

	addExtraCortexOpsCmd(dryrun.BuildCmd("config dry-run", CortexOpsContextInjector,
		BuildCortexOpsSetConfigurationCmd(),
		BuildCortexOpsSetDefaultConfigurationCmd(),
		BuildCortexOpsResetConfigurationCmd(),
		BuildCortexOpsResetDefaultConfigurationCmd(),
		BuildCortexOpsInstallCmd(),
		BuildCortexOpsUninstallCmd(),
	))
	addExtraCortexOpsCmd(rollback.BuildCmd("config rollback", CortexOpsContextInjector))
}
