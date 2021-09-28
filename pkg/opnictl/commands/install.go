package commands

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/mattn/go-isatty"
	"github.com/rancher/opni/pkg/features"
	"github.com/rancher/opni/pkg/opnictl/common"
	cliutil "github.com/rancher/opni/pkg/util/opnictl"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	"github.com/vbauerster/mpb/v7"
	"github.com/vbauerster/mpb/v7/decor"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

func BuildInstallCmd() *cobra.Command {
	var interactive bool
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install Opni Manager",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			featureGates := cmd.Flag("feature-gates").Value.String()
			// Set the feature gates into a temporary copy to verify the flag is
			// well-formed; this will not actually set the feature gates.
			return features.DefaultMutableFeatureGate.DeepCopy().Set(featureGates)
		},
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
		RunE: func(cmd *cobra.Command, args []string) error {
			if interactive {
				var selectedGates []string
				gpuGateDesc := "Automatic GPU Configuration"
				err := survey.AskOne(&survey.MultiSelect{
					Message: "Select optional features to enable",
					Options: []string{gpuGateDesc},
					Default: []string{},
				}, &selectedGates)
				if err != nil {
					return err
				}
				for _, gate := range selectedGates {
					if gate == gpuGateDesc {
						err := features.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
							string(features.GPUOperator):                  true,
							string(features.NodeFeatureDiscoveryOperator): true,
						})
						if err != nil {
							panic(fmt.Sprintf("bug: error setting feature gates: %s", err.Error()))
						}
						break
					}
				}
			}
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
				msgs = cliutil.ForEachStagingResource(
					common.RestConfig,
					func(dr dynamic.ResourceInterface, obj *unstructured.Unstructured) error {
						if obj.GetKind() == "Deployment" && obj.GetName() == "opni-controller-manager" {
							if err := features.PatchFeatureGatesInWorkload(obj, "manager"); err != nil {
								return err
							}
						}
						_, err := dr.Create(
							cmd.Context(),
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
			return nil
		},
	}

	installCmd.Flags().BoolVar(&interactive, "interactive", isatty.IsTerminal(os.Stdout.Fd()), "use interactive prompts (disabling this will use only defaults)")
	features.DefaultMutableFeatureGate.AddFlag(installCmd.Flags())
	return installCmd
}
