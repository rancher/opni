package commands

import (
	"context"
	"fmt"

	"github.com/rancher/opni/pkg/opnictl/common"
	"github.com/ttacon/chalk"

	"github.com/rancher/opni/apis/demo/v1alpha1"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func BuildGetCmd() *cobra.Command {
	return &cobra.Command{
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
				getOpniDemoes(cmd.Context())
			default:
				common.Log.Fatalf("Unknown resource %s", args[0])
			}
		},
	}
}

func getOpniDemoes(ctx context.Context) error {
	list := &v1alpha1.OpniDemoList{}

	if err := common.K8sClient.List(ctx, list, client.InNamespace(common.NamespaceFlagValue)); err != nil {
		return err
	}

	if len(list.Items) == 0 {
		return fmt.Errorf("no resources found in %s namespace", common.NamespaceFlagValue)
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
	return nil
}
