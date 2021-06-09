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
	Use:   "get [opnidemoes|opniclusters]",
	Short: "Show existing opni resources",
	Long: `
The get command will list opni Custom Resource objects that exist in your
cluster. For each object listed, its status will be shown. If the resources
controlled by the object are healthy and running, it will display "Ready", 
otherwise it will display "Waiting" and one or more status conditions will be
displayed for that object.

Your current kubeconfig context will be used to select the cluster to operate
on, unless the --context flag is provided to select a specific context.`,
	Args:       cobra.ExactArgs(1),
	ValidArgs:  []string{"opnidemoes"},
	ArgAliases: []string{"demoes", "demos", "opnidemo", "opnidemos"},
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "opnidemoes", "demoes", "demos", "opnidemo", "opnidemos":
			getOpniDemoes()
		default:
			Log.Fatalf("Unknown resource %s", args[0])
		}
	},
}

func getOpniDemoes() {
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
}
