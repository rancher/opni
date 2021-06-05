package commands

import (
	"context"
	"fmt"
	"os"

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
	Run: func(cmd *cobra.Command, args []string) {
		p := mpb.New()

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
			msgs = ForEachStagingResource(func(dr dynamic.ResourceInterface, obj *unstructured.Unstructured) error {
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
