package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb"
	"github.com/vbauerster/mpb/decor"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

var UninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall Opni Manager",
	Run: func(cmd *cobra.Command, args []string) {
		p := mpb.New()

		spinner := p.AddSpinner(1, mpb.SpinnerOnLeft,
			mpb.PrependDecorators(
				decor.OnComplete(decor.Name("Uninstalling Opni Resources..."), "Uninstall completed."),
			),
			mpb.BarClearOnComplete(),
		)
		var msgs []string
		go func() {
			msgs = ForEachStagingResource(func(dr dynamic.ResourceInterface, obj *unstructured.Unstructured) error {
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
