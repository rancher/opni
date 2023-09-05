package cortexops

import (
	"fmt"
	strings "strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/nsf/jsondiff"
	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	cliutil "github.com/rancher/opni/pkg/opni/cliutil"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/util/flagutil"
	cobra "github.com/spf13/cobra"
)

func BuildRollbackCmd() *cobra.Command {
	var revision *int64
	var target string
	cmd := &cobra.Command{
		Use: "config rollback",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ok := CortexOpsClientFromContext(cmd.Context())
			if !ok {
				cmd.PrintErrln("failed to get client from context")
				return nil
			}

			// look up both the current and target revisions
			var currentConfig, targetConfig *CapabilityBackendConfigSpec
			switch target {
			case "ActiveConfiguration":
				var err error
				currentConfig, err = client.GetConfiguration(cmd.Context(), &GetRequest{})
				if err != nil {
					return err
				}
				targetConfig, err = client.GetConfiguration(cmd.Context(), &GetRequest{
					Revision: v1.NewRevision(*revision),
				})
				if err != nil {
					return err
				}
			case "DefaultConfiguration":
				var err error
				currentConfig, err = client.GetDefaultConfiguration(cmd.Context(), &GetRequest{})
				if err != nil {
					return err
				}
				targetConfig, err = client.GetDefaultConfiguration(cmd.Context(), &GetRequest{
					Revision: v1.NewRevision(*revision),
				})
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("invalid target %q", target)
			}
			if currentConfig.GetRevision().GetRevision() == targetConfig.GetRevision().GetRevision() {
				return fmt.Errorf("current configuration is already at revision %d", *revision)
			}

			// show the pending changes
			diffStr, _ := driverutil.RenderJsonDiff(currentConfig, targetConfig, jsondiff.DefaultConsoleOptions())
			cmd.Println(diffStr)

			// prompt for confirmation
			confirm := false
			if err := survey.AskOne(&survey.Confirm{
				Message: fmt.Sprintf("Rollback the %s configuration to revision %d?",
					strings.ToLower(strings.TrimSuffix(target, "Configuration")), *revision),
				Default: false,
			}, &confirm); err != nil {
				return err
			}
			if !confirm {
				return fmt.Errorf("rollback aborted")
			}

			// perform the rollback
			targetConfig.Revision = currentConfig.Revision
			var err error
			switch target {
			case "ActiveConfiguration":
				_, err = client.SetConfiguration(cmd.Context(), targetConfig)
			case "DefaultConfiguration":
				_, err = client.SetDefaultConfiguration(cmd.Context(), targetConfig)
			}
			if err != nil {
				return fmt.Errorf("rollback failed (no changes were made): %w", err)
			}
			cmd.Printf("successfully rolled back to revision %d\n", *revision)
			return nil
		},
	}
	cmd.Flags().Var(flagutil.IntPtrValue(nil, &revision), "revision", "revision to rollback to")
	cmd.Flags().StringVar(&target, "target", "ActiveConfiguration", "the configuration type to rollback")
	cmd.MarkFlagRequired("revision")
	cmd.MarkFlagRequired("target")

	cmd.RegisterFlagCompletionFunc("target", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"ActiveConfiguration", "DefaultConfiguration"}, cobra.ShellCompDirectiveDefault
	})
	cmd.RegisterFlagCompletionFunc("revision", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		cliutil.BasePreRunE(cmd, args)
		return completeRevisions(cmd.Context(), driverutil.Target(driverutil.Target_value[target]))
	})
	return cmd
}
