package commands

import (
	"context"
	"fmt"
	"os"

	. "github.com/rancher/opni/pkg/opnictl/common"

	"github.com/rancher/opni/pkg/util/opnictl"
	cliutil "github.com/rancher/opni/pkg/util/opnictl"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

var UninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall Opni",
	Long: `
The Uninstall command will completely uninstall Opni from your cluster. This 
includes the Manager, as well as any Opni services that were created using opnictl.

Your current kubeconfig context will be used to select the cluster to uninstall 
Opni from, unless the --context flag is provided to select a specific context.`,
	Run: func(cmd *cobra.Command, args []string) {
		p := mpb.New()

		clientConfig := opnictl.LoadClientConfig(MaybeContextOverride()...)

		spinner := p.AddSpinner(1,
			mpb.AppendDecorators(
				decor.OnComplete(
					decor.Name(chalk.Bold.TextStyle("Uninstalling Opni Resources..."), decor.WCSyncSpaceR),
					chalk.Bold.TextStyle("Uninstall completed."),
				),
			),
			mpb.BarFillerOnComplete(chalk.Green.Color("✓")),
			mpb.BarWidth(1),
		)
		var msgs []string
		go func() {
			msgs = cliutil.ForEachStagingResource(
				clientConfig,
				func(dr dynamic.ResourceInterface, obj *unstructured.Unstructured) error {
					return dr.Delete(
						context.Background(),
						obj.GetName(),
						v1.DeleteOptions{},
					)
				})
			spinner.Increment()
		}()
		p.Wait()

		for _, msg := range msgs {
			fmt.Fprintln(os.Stderr, msg)
		}
	},
}
