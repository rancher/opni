package commands

import (
	"context"
	"fmt"

	. "github.com/rancher/opni/pkg/opnictl/common"
	"github.com/ttacon/chalk"

	"github.com/rancher/opni/api/v1alpha1"
	cliutil "github.com/rancher/opni/pkg/util/opnictl"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var GetCmd = &cobra.Command{
	Use:   "get resource",
	Short: "Show existing opni resources",
}

var GetDemoCmd = &cobra.Command{
	Use:   "demo-cluster name",
	Args:  cobra.NoArgs,
	Short: "Show existing OpniDemo resources",
	Run: func(cmd *cobra.Command, args []string) {
		cli := cliutil.CreateClientOrDie()

		list := &v1alpha1.OpniDemoList{}

		if err := cli.List(context.Background(), list, client.InNamespace(NamespaceFlagValue)); err != nil {
			Log.Fatal(err)
		}

		if len(list.Items) == 0 {
			fmt.Printf("No resources found in %s namespace.\n", NamespaceFlagValue)
			return
		}

		for _, demo := range list.Items {
			fmt.Printf("%s [%s]\n", chalk.Bold.TextStyle(demo.Name), func() string {
				switch demo.Status.State {
				case "Waiting":
					return chalk.Yellow.Color("Waiting")
				case "Ready":
					return chalk.Green.Color("Ready")
				}
				return chalk.Dim.TextStyle("Unknown")
			}())
			for _, cond := range demo.Status.Conditions {
				fmt.Printf("  %s\n", chalk.Yellow.Color(cond))
			}
		}
	},
}

func init() {
	GetCmd.AddCommand(GetDemoCmd)
}
