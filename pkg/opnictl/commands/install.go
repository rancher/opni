package commands

import (
	"context"
	"fmt"
	"os"

	. "github.com/rancher/opni/pkg/opnictl/common"
	cliutil "github.com/rancher/opni/pkg/util/opnictl"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

var InstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Opni Manager",
	Long: `
The install command will install the Opni Manager (operator) into your cluster, 
along with the Opni CRDs. 

Your current kubeconfig context will be used to select the cluster to install 
Opni into, unless the --context flag is provided to select a specific context.

A namespace can be provided using the --namespace flag, otherwise a default 
namespace of 'opni-system' will be used. If the opni-system namespace does not
exist, it will be created, but if a custom namespace is specified, it must
already exist.

Once the manager is running, install the Opni services using one of the Opni
APIs. For more information on selecting an API, run 'opnictl help apis'.`,
	Run: func(cmd *cobra.Command, args []string) {
		p := mpb.New()

		clientConfig := cliutil.LoadClientConfig(MaybeContextOverride()...)

		spinner := p.AddSpinner(1,
			mpb.AppendDecorators(
				decor.OnComplete(
					decor.Name(chalk.Bold.TextStyle("Installing Opni Resources..."), decor.WCSyncSpaceR),
					chalk.Bold.TextStyle("Installation completed."),
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
					_, err := dr.Create(
						context.Background(),
						obj,
						v1.CreateOptions{},
					)
					return err
				})
			spinner.Increment()
		}()
		p.Wait()

		for _, msg := range msgs {
			fmt.Fprintln(os.Stderr, msg)
		}
	},
}
