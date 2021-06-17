package commands

import (
	"context"

	. "github.com/rancher/opni/pkg/opnictl/common"
	"go.uber.org/atomic"

	"github.com/rancher/opni/api/v1alpha1"
	cliutil "github.com/rancher/opni/pkg/util/opnictl"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
	"k8s.io/apimachinery/pkg/types"
)
func BuildDeleteDemoCmd() *cobra.Command {
	var deleteDemoCmd = &cobra.Command{
		Use:   "demo name",
		Args:  cobra.ExactArgs(1),
		Short: "Delete an existing opni demo cluster",
		Long: `
This command will remove an installation of Opni from the selected namespace.
Any installations of Opni in other namespaces, as well as the Opni Manager and
CRDs, will remain.

Your current kubeconfig context will be used to select the cluster to operate
on, unless the --context flag is provided to select a specific context.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			demo := &v1alpha1.OpniDemo{}
			if err := K8sClient.Get(cmd.Context(), types.NamespacedName{
				Namespace: NamespaceFlagValue,
				Name:      args[0],
			}, demo); err != nil {
				return err
			}

			p := mpb.New()
			waitCtx, ca := context.WithTimeout(cmd.Context(), TimeoutFlagValue)
			defer ca()
			deleteError := atomic.NewError(nil)
			waitingSpinner := p.AddSpinner(1,
				mpb.AppendDecorators(
					decor.OnComplete(decor.Name(chalk.Bold.TextStyle("Deleting resources..."), decor.WCSyncSpaceR),
						chalk.Bold.TextStyle("Done."),
					),
				),
				mpb.BarFillerMiddleware(
					cliutil.CheckBarFiller(waitCtx, func(c context.Context) bool {
						return waitCtx.Err() == nil && deleteError.Load() == nil
					})),
				mpb.BarWidth(1),
			)

			var asyncErr error
			go func() {
				defer waitingSpinner.Increment()
				deleteError.Store(K8sClient.Delete(waitCtx, demo))
				if err := deleteError.Load(); err != nil {
					asyncErr = err
				}
			}()

			p.Wait()
			return asyncErr
		},
	}
	return deleteDemoCmd
}

func BuildDeleteCmd() *cobra.Command {
	var deleteCmd = &cobra.Command{
		Use:   "delete resource",
		Short: "Delete existing opni resources",
		Long:  "See subcommands for more information.",
	}
	 
	deleteCmd.AddCommand(BuildDeleteDemoCmd())

	return deleteCmd
}
