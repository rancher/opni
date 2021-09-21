package commands

import (
	"fmt"
	"net/url"

	"github.com/AlecAivazis/survey/v2"
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/opnictl/common"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func BuildCreateClusterCmd() *cobra.Command {
	var (
		edit                    bool
		name                    string
		version                 string
		interactive             bool
		controlPlaneModelSource string
		controlPlaneModelImage  string
		controlPlaneModelURL    string
	)
	defaultModelURL := "https://opni-public.s3.us-east-2.amazonaws.com/pretrain-models/control-plane-model-v0.1.2.zip"

	var createClusterCmd = &cobra.Command{
		Use:   "cluster",
		Short: "Create a new opni cluster",
		Args:  cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Check if control plane model source is correct
			switch controlPlaneModelSource {
			case "http":
				if controlPlaneModelURL == "" && !interactive {
					return fmt.Errorf("Missing required flag: --control-plane-model-url (non-interactive mode)")
				}
			case "image":
				if controlPlaneModelImage == "" && !interactive {
					return fmt.Errorf("Missing required flag: --control-plane-model-image (non-interactive mode)")
				}
			default:
				return fmt.Errorf("Invalid control plane model source: %s (acceptable values: \"http\", \"image\")", controlPlaneModelSource)
			}
			return nil
		},
		Long: fmt.Sprintf(`
		This command will install opni into the selected namespace using the Production API.
		For more information about the Production API, run %s.
		
		Your current kubeconfig context will be used to select the cluster to operate
		on, unless the --context flag is provided to select a specific context.`,
			chalk.Bold.TextStyle("opnictl help apis")),
		RunE: func(cmd *cobra.Command, args []string) error {
			pretrainedModel := &v1beta1.PretrainedModel{
				ObjectMeta: v1.ObjectMeta{
					Name:      "control-plane",
					Namespace: common.NamespaceFlagValue,
				},
			}
			switch controlPlaneModelSource {
			case "http":
				if controlPlaneModelURL == "" {
					// interactive check is done in PreRunE
					err := survey.AskOne(
						&survey.Input{
							Message: "Enter a URL for the control-plane pretrained model",
							Default: defaultModelURL,
						},
						&controlPlaneModelURL,
						survey.WithValidator(survey.ComposeValidators(
							survey.Required,
							func(ans interface{}) error {
								url, err := url.Parse(ans.(string))
								if err != nil {
									return err
								}
								if url.Scheme != "http" && url.Scheme != "https" {
									return fmt.Errorf("URL scheme must be http or https")
								}
								return nil
							},
						)),
					)
					if err != nil {
						return err
					}
				}
				pretrainedModel.Spec = v1beta1.PretrainedModelSpec{
					ModelSource: v1beta1.ModelSource{
						HTTP: &v1beta1.HTTPSource{
							URL: controlPlaneModelURL,
						},
					},
				}
			case "image":
				if controlPlaneModelImage == "" {
					// interactive check is done in PreRunE
					err := survey.AskOne(
						&survey.Input{
							Message: "Enter an image for the control-plane pretrained model",
						},
						&controlPlaneModelImage,
						survey.WithValidator(survey.Required),
					)
					if err != nil {
						return err
					}
				}
				pretrainedModel.Spec = v1beta1.PretrainedModelSpec{
					ModelSource: v1beta1.ModelSource{
						Container: &v1beta1.ContainerSource{
							Image: controlPlaneModelImage,
						},
					},
				}
			}

			opniCluster := &v1beta1.OpniCluster{
				ObjectMeta: v1.ObjectMeta{
					Name:      name,
					Namespace: common.NamespaceFlagValue,
				},
				Spec: v1beta1.OpniClusterSpec{
					Version:            version,
					DeployLogCollector: pointer.Bool(true),
					Services: v1beta1.ServicesSpec{
						Inference: v1beta1.InferenceServiceSpec{
							PretrainedModels: []corev1.LocalObjectReference{
								{
									Name: "control-plane",
								},
							},
						},
					},
				},
			}

			return nil
		},
	}

	createClusterCmd.Flags().StringVar(&name, "name", "", "resource name")
	createClusterCmd.Flags().StringVar(&version, "version", "latest", "opni version to install")
	createClusterCmd.Flags().BoolVar(&edit, "edit", false, "edit resource before creating it")
	createClusterCmd.Flags().BoolVar(&interactive, "interactive", true, "use interactive prompts (disabling this will use only defaults)")
	createClusterCmd.Flags().StringVar(&controlPlaneModelSource, "control-plane-model-source", "http", "control plane model source (http|image)")
	createClusterCmd.Flags().StringVar(&controlPlaneModelURL, "control-plane-model-url", "", "optional alternative control plane model URL")
	createClusterCmd.Flags().StringVar(&controlPlaneModelImage, "control-plane-model-image", "", "control plane model image (if source is \"image\")")

	createClusterCmd.MarkFlagRequired("name")

	return createClusterCmd
}
