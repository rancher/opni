package rollback

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/nsf/jsondiff"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/opni/cliutil"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/driverutil/complete"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/flagutil"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protorange"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Builds a rollback command given a use string and a function that returns a
// new typed client from a context (this function is usually generated).
//
//	rollback.BuildCmd("rollback", NewXClientFromContext)
func BuildCmd[
	C driverutil.Client[T, R, D, DR, HR],
	T driverutil.ConfigType[T],
	R driverutil.ResetRequestType[T],
	D driverutil.DryRunRequestType[T],
	DR driverutil.DryRunResponseType[T],
	HR driverutil.HistoryResponseType[T],
](use string, newClientFunc func(context.Context) (C, bool)) *cobra.Command {
	var (
		revision *int64
		target   driverutil.Target
	)
	cmd := &cobra.Command{
		Use:   use,
		Short: `Revert the active or default configuration to a previous revision.`,
		Long: `
Revert the active or default configuration to a previous revision.

To easily identify the revision you want to rollback to, use the "history" command
to view a list of previous revisions and their associated diffs. Note that the
diff displayed alongside each revision is compared to its previous revision,
not the current configuration; when rolling back to that revision, the changes
displayed in the diff will be included in the rollback.

Before performing the rollback, you will get a chance to view the pending changes,
and make edits to the configuration if desired.

If the target revision contains secrets that have since been cleared (referred
to as a "discontinuity"), you will be prompted to set new values for all secret
fields that were present in the target revision.

For example, if revision 1 has a stored value for a secret and revision 2 was
created such that the field containing the secret was cleared or reset, then
rolling back to revision 1 will cause a discontinuity error. Because secrets are
only read by the client as redacted placeholder values, there is no way for the
client to know what the original value of the secret was.

However, because the "rollback" operation is simply applying changes on top of
the current configuration in a specific way, if both the current and target
revisions have stored values for all the same secret fields, this does not
constitute a discontinuity and the rollback will proceed as normal, except that
the secret values will not change from the current configuration.
`[1:],
		RunE: func(cmd *cobra.Command, args []string) error {
			client, ok := newClientFunc(cmd.Context())
			if !ok {
				cmd.PrintErrln("failed to get client from context")
				return nil
			}

			// look up both the current and target revisions
			var currentConfig, targetConfig T

			switch target {
			case driverutil.Target_ActiveConfiguration:
				var err error
				currentConfig, err = client.GetConfiguration(cmd.Context(), &driverutil.GetRequest{})
				if err != nil {
					return err
				}
				targetConfig, err = client.GetConfiguration(cmd.Context(), &driverutil.GetRequest{
					Revision: corev1.NewRevision(*revision),
				})
				if err != nil {
					return err
				}
			case driverutil.Target_DefaultConfiguration:
				var err error
				currentConfig, err = client.GetDefaultConfiguration(cmd.Context(), &driverutil.GetRequest{})
				if err != nil {
					return err
				}
				targetConfig, err = client.GetDefaultConfiguration(cmd.Context(), &driverutil.GetRequest{
					Revision: corev1.NewRevision(*revision),
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
			driverutil.CopyRevision(targetConfig, currentConfig)

			for {
				dryRunReq := util.NewMessage[D]()
				{
					rm := dryRunReq.ProtoReflect()
					rmd := rm.Descriptor()
					rm.Set(rmd.Fields().ByName("target"), protoreflect.ValueOfEnum(target.Number()))
					rm.Set(rmd.Fields().ByName("action"), protoreflect.ValueOfEnum(driverutil.Action_Set.Number()))
					rm.Set(rmd.Fields().ByName("spec"), protoreflect.ValueOfMessage(targetConfig.ProtoReflect()))
				}

				dryRunResp, err := client.DryRun(cmd.Context(), dryRunReq)
				if err != nil {
					if storage.IsDiscontinuity(err) {
						// In this case, the user is trying to rollback to a revision that
						// contained secrets that have since been cleared. We could revert
						// the secrets back to the values present in the target revision,
						// but that technically breaks an API contract and would need
						// special-case logic on the server side to handle (plus there is
						// likely a good reason the secrets were cleared in the first place).
						// Instead, prompt the user to fill in values for the missing secrets,
						// then retry.
						cmd.Println(chalk.Yellow.Color("The target revision contains redacted secrets that have since been cleared. Follow the prompts below to fill in the missing values."))
						allFields := []string{}
						for _, detail := range status.Convert(err).Details() {
							if info, ok := detail.(*errdetails.ErrorInfo); ok && info.Reason == "DISCONTINUITY" {
								allFields = append(allFields, info.Metadata["field"])
							}
						}
						slices.Sort(allFields)
						questions := []*survey.Question{}
						answers := make(map[string]any)
						for _, field := range allFields {
							questions = append(questions, &survey.Question{
								Name: field,
								Prompt: &survey.Password{
									Message: fmt.Sprintf("Enter value for %s: ", field),
								},
								Validate: survey.Required,
							})
						}
						if len(questions) == 0 {
							panic("bug: secrets discontinuity error missing field metadata")
						}
						if err := survey.Ask(questions, &answers); err != nil {
							return fmt.Errorf("rollback canceled: %w", err)
						}
						protorange.Range(targetConfig.ProtoReflect(), func(vs protopath.Values) error {
							v := vs.Index(-1)
							if v.Step.Kind() != protopath.FieldAccessStep {
								return nil
							}
							fd := v.Step.FieldDescriptor()
							if fd.Kind() == protoreflect.StringKind && !fd.IsList() && !fd.IsMap() {
								answerKey := vs.Path[1:].String()[1:]
								if answer, ok := answers[answerKey]; ok {
									containingMsg := vs.Index(-2).Value.Message()
									containingMsg.Set(fd, protoreflect.ValueOfString(answer.(string)))
								}
							}
							return nil
						})
						continue
					}
					return fmt.Errorf("dry-run failed: %w", err)
				}

				diffStr, anyChanges := driverutil.RenderJsonDiff(dryRunResp.GetCurrent(), dryRunResp.GetModified(), jsondiff.DefaultConsoleOptions())
				if !anyChanges {
					cmd.Println(chalk.Green.Color("No changes to apply."))
					return nil
				}
				cmd.Printf("The following changes will be applied (%s):\n", driverutil.DiffStat(diffStr))
				cmd.Println(diffStr)

				// prompt for confirmation
				message := fmt.Sprintf("Rollback the %s configuration to revision %d?",
					strings.ToLower(strings.TrimSuffix(target.String(), "Configuration")), *revision)
				yes := "Yes"

				comments := []string{}
				if len(dryRunResp.GetValidationErrors()) > 0 {
					yes += " (bypass validation checks)"
					comments = append(comments, "Validation warnings:")
				}
				for _, e := range dryRunResp.GetValidationErrors() {
					switch e.Severity {
					case driverutil.ValidationError_Error:
						comments = append(comments, fmt.Sprintf(" - %s: %s", e.GetSeverity(), e.GetMessage()))
					case driverutil.ValidationError_Warning:
						comments = append(comments, fmt.Sprintf(" - %s: %s", e.GetSeverity(), e.GetMessage()))
					}
				}
				for _, comment := range comments {
					cmd.Println(chalk.Yellow.Color(comment))
				}
				var confirm string
				if err := survey.AskOne(&survey.Select{
					Message: message,
					Options: []string{
						yes,
						"No",
						"Edit",
					},
					Default: "No",
				}, &confirm); err != nil {
					return err
				}
				switch confirm {
				case "No":
					return fmt.Errorf("rollback canceled")
				case "Edit":
					if cfg, err := cliutil.EditInteractive(targetConfig, comments...); err != nil {
						return err
					} else {
						targetConfig = cfg
						continue
					}
				case yes:
					if len(dryRunResp.GetValidationErrors()) > 0 {
						var confirm bool
						if err := survey.AskOne(&survey.Confirm{
							Message: "This will bypass validation checks. The configuration may not function correctly. Are you sure?",
							Default: false,
						}, &confirm); err != nil {
							return err
						}
						if !confirm {
							return fmt.Errorf("rollback canceled")
						}
					}
				default:
					panic("bug: unexpected response " + confirm)
				}

				// perform the rollback
				switch target {
				case driverutil.Target_ActiveConfiguration:
					// reset using a mask that includes all present fields in the target config,
					// and the target config as the patch.
					resetReq := util.NewMessage[R]()
					mask := util.NewFieldMaskByPresence(targetConfig.ProtoReflect())

					rm := resetReq.ProtoReflect()
					rmd := rm.Descriptor()
					rm.Set(rmd.Fields().ByName("mask"), protoreflect.ValueOfMessage(mask.ProtoReflect()))
					rm.Set(rmd.Fields().ByName("patch"), protoreflect.ValueOfMessage(targetConfig.ProtoReflect()))

					_, err = client.ResetConfiguration(cmd.Context(), resetReq)
				case driverutil.Target_DefaultConfiguration:
					_, err = client.SetDefaultConfiguration(cmd.Context(), targetConfig)
				}
				if err != nil {
					cmd.PrintErrln("rollback failed:", err)
				}
				cmd.Printf("successfully rolled back to revision %d\n", *revision)
				return nil
			}
		},
	}
	cmd.Flags().Var(flagutil.IntPtrValue(nil, &revision), "revision", "revision to rollback to")
	cmd.Flags().Var(flagutil.EnumValue(&target), "target", "the configuration type to rollback")
	cmd.MarkFlagRequired("revision")

	cmd.RegisterFlagCompletionFunc("target", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"ActiveConfiguration", "DefaultConfiguration"}, cobra.ShellCompDirectiveDefault
	})
	cmd.RegisterFlagCompletionFunc("revision", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		cliutil.BasePreRunE(cmd, args)
		client, ok := newClientFunc(cmd.Context())
		if !ok {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return complete.Revisions(cmd.Context(), target, client)
	})
	return cmd
}
