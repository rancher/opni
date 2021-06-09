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

var DeleteCmd = &cobra.Command{
	Use:   "delete resource",
	Short: "Delete existing opni resources",
}

var DeleteDemoCmd = &cobra.Command{
	Use:   "demo-cluster name",
	Args:  cobra.ExactArgs(1),
	Short: "Delete an existing opni demo cluster",
	Run: func(cmd *cobra.Command, args []string) {
		cli := cliutil.CreateClientOrDie()

		demo := &v1alpha1.OpniDemo{}
		if err := cli.Get(context.Background(), types.NamespacedName{
			Namespace: NamespaceFlagValue,
			Name:      args[0],
		}, demo); err != nil {
			Log.Fatal(err)
		}

		p := mpb.New()
		waitCtx, ca := context.WithTimeout(context.Background(), TimeoutFlagValue)
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

		go func() {
			defer waitingSpinner.Increment()
			deleteError.Store(cli.Delete(waitCtx, demo))
			if err := deleteError.Load(); err != nil {
				Log.Fatal(err)
			}
		}()

		p.Wait()
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteDemoCmd)
}
