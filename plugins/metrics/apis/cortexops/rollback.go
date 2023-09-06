package cortexops

import (
	"fmt"
	"slices"
	strings "strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/nsf/jsondiff"
	v1 "github.com/rancher/opni/pkg/apis/core/v1"
	cliutil "github.com/rancher/opni/pkg/opni/cliutil"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util/flagutil"
	cobra "github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	errdetails "google.golang.org/genproto/googleapis/rpc/errdetails"
	status "google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protorange"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
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
			targetConfig.Revision = currentConfig.Revision

			// show the pending changes
		DRY_RUN:
			dryRunResp, err := client.DryRun(cmd.Context(), &DryRunRequest{
				Target: driverutil.Target(driverutil.Target_value[target]),
				Action: driverutil.Action_Set,
				Spec:   targetConfig,
			})
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
					goto DRY_RUN
				}
				return fmt.Errorf("dry-run failed: %w", err)
			}

			diffStr, _ := driverutil.RenderJsonDiff(dryRunResp.Current, dryRunResp.Modified, jsondiff.DefaultConsoleOptions())
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
