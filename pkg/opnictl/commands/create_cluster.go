package commands

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	nfdv1 "github.com/kubernetes-sigs/node-feature-discovery-operator/api/v1"
	"github.com/mattn/go-isatty"
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/features"
	"github.com/rancher/opni/pkg/opnictl/common"
	"github.com/rancher/opni/pkg/providers"
	cliutil "github.com/rancher/opni/pkg/util/opnictl"
	"github.com/spf13/cobra"
	"github.com/ttacon/chalk"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type flagVars struct {
	edit                    bool
	name                    string
	version                 string
	provider                string
	interactive             bool
	noPretrainedModel       bool
	noLogAdapter            bool
	noNodeFeatureDiscovery  bool
	noGpuPolicyAdapter      bool
	controlPlaneModelSource string
	controlPlaneModelImage  string
	controlPlaneModelURL    string
}

var (
	defaultModelURL = "https://opni-public.s3.us-east-2.amazonaws.com/pretrain-models/control-plane-model-v0.1.2.zip"
)

func buildPretrainedModel(cmd *cobra.Command, vars *flagVars) (*v1beta1.PretrainedModel, error) {
	pretrainedModel := &v1beta1.PretrainedModel{
		ObjectMeta: v1.ObjectMeta{
			Name:      "control-plane",
			Namespace: common.NamespaceFlagValue,
		},
	}
	switch vars.controlPlaneModelSource {
	case "http":
		if vars.controlPlaneModelURL == "" {
			// interactive check is done in PreRunE
			err := survey.AskOne(
				&survey.Input{
					Message: "Enter a URL for the control-plane pretrained model",
					Default: defaultModelURL,
				},
				&vars.controlPlaneModelURL,
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
				return nil, err
			}
		}
		pretrainedModel.Spec = v1beta1.PretrainedModelSpec{
			ModelSource: v1beta1.ModelSource{
				HTTP: &v1beta1.HTTPSource{
					URL: vars.controlPlaneModelURL,
				},
			},
		}
	case "image":
		if vars.controlPlaneModelImage == "" {
			// interactive check is done in PreRunE
			err := survey.AskOne(
				&survey.Input{
					Message: "Enter an image for the control-plane pretrained model",
				},
				&vars.controlPlaneModelImage,
				survey.WithValidator(survey.Required),
			)
			if err != nil {
				return nil, err
			}
		}
		pretrainedModel.Spec = v1beta1.PretrainedModelSpec{
			ModelSource: v1beta1.ModelSource{
				Container: &v1beta1.ContainerSource{
					Image: vars.controlPlaneModelImage,
				},
			},
			Hyperparameters: map[string]intstr.IntOrString{
				"modelThreshold": intstr.FromString("0.6"),
				"minLogTokens":   intstr.FromInt(4),
			},
		}
	}
	return pretrainedModel, nil
}

func buildOpniCluster(vars *flagVars) *v1beta1.OpniCluster {
	return &v1beta1.OpniCluster{
		ObjectMeta: v1.ObjectMeta{
			Name:      vars.name,
			Namespace: common.NamespaceFlagValue,
		},
		Spec: v1beta1.OpniClusterSpec{
			Version:            vars.version,
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
			Elastic: v1beta1.ElasticSpec{
				Version: "1.13.2",
			},
			S3: v1beta1.S3Spec{
				Internal: &v1beta1.InternalSpec{
					Persistence: &v1beta1.PersistenceSpec{
						Enabled: true,
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Request: resource.MustParse("10Gi"),
					},
				},
			},
			Nats: v1beta1.NatsSpec{
				AuthMethod: v1beta1.NatsAuthNkey,
			},
		},
	}
}

func buildLogAdapter(vars *flagVars) *v1beta1.LogAdapter {
	return &v1beta1.LogAdapter{
		ObjectMeta: v1.ObjectMeta{
			Name:      "lga",
			Namespace: common.NamespaceFlagValue,
		},
		Spec: v1beta1.LogAdapterSpec{
			Provider: v1beta1.LogProvider(vars.provider),
			OpniCluster: v1beta1.OpniClusterNameSpec{
				Name:      vars.name,
				Namespace: common.NamespaceFlagValue,
			},
		},
	}
}

func buildGpuPolicyAdapter(vars *flagVars) *v1beta1.GpuPolicyAdapter {
	return &v1beta1.GpuPolicyAdapter{
		ObjectMeta: v1.ObjectMeta{
			Name:      "gpu-policy",
			Namespace: common.NamespaceFlagValue,
		},
		Spec: v1beta1.GpuPolicyAdapterSpec{
			ContainerRuntime:   "auto",
			KubernetesProvider: "auto",
		},
	}
}

func buildNodeFeatureDiscovery(vars *flagVars) *nfdv1.NodeFeatureDiscovery {
	return &nfdv1.NodeFeatureDiscovery{
		ObjectMeta: v1.ObjectMeta{
			Name:      "opni-nfd-server",
			Namespace: common.NamespaceFlagValue,
		},
		Spec: nfdv1.NodeFeatureDiscoverySpec{
			Operand: nfdv1.OperandSpec{
				Namespace:       common.NamespaceFlagValue,
				Image:           "k8s.gcr.io/nfd/node-feature-discovery:v0.7.0",
				ImagePullPolicy: "IfNotPresent",
				ServicePort:     12000,
			},
			ExtraLabelNs: []string{"nvidia.com"},
			WorkerConfig: nfdv1.ConfigMap{
				ConfigData: `---
sources:
	pci:
		deviceLabelFields:
		- vendor`,
			},
		},
	}
}

func ignoreAlreadyExists(err error) error {
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func BuildCreateClusterCmd() *cobra.Command {
	vars := &flagVars{}

	var createClusterCmd = &cobra.Command{
		Use:   "cluster",
		Short: "Create a new opni cluster",
		Args:  cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Check if control plane model source is correct
			switch vars.controlPlaneModelSource {
			case "http":
				if vars.controlPlaneModelURL == "" && !vars.interactive {
					vars.controlPlaneModelURL = defaultModelURL
				}
			case "image":
				if vars.controlPlaneModelImage == "" && !vars.interactive {
					return fmt.Errorf("Missing required flag: --control-plane-model-image (non-interactive mode)")
				}
			default:
				return fmt.Errorf("Invalid control plane model source: %s (acceptable values: \"http\", \"image\")", vars.controlPlaneModelSource)
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
			// Look up the feature gates used by the opni controller
			controllers := appsv1.DeploymentList{}
			labelReq, err := labels.NewRequirement(
				"app",
				selection.In,
				[]string{"opni-controller-manager"},
			)
			if err != nil {
				panic(err)
			}
			err = common.K8sClient.List(context.Background(), &controllers, &client.ListOptions{
				Namespace:     "opni-system",
				LabelSelector: labels.NewSelector().Add(*labelReq),
			})
			if err != nil {
				return err
			}
			if len(controllers.Items) == 0 {
				return fmt.Errorf("No opni controller manager found in namespace opni-system")
			}
			if len(controllers.Items) > 1 {
				return fmt.Errorf("Multiple opni controller managers found in namespace opni-system")
			}
			controller := controllers.Items[0]
			containerArgs := controller.Spec.Template.Spec.Containers[0].Args
			featureGate := features.DefaultMutableFeatureGate.DeepCopy()
			for _, arg := range containerArgs {
				if strings.HasPrefix(arg, "--feature-gates=") {
					if err := featureGate.Set(strings.TrimPrefix(arg, "--feature-gates=")); err != nil {
						return err
					}
					break
				}
			}

			if vars.provider == "" {
				provider, err := providers.Detect(cmd.Context(), common.K8sClient)
				if err != nil {
					return err
				}
				vars.provider = provider.String()
			}
			if common.NamespaceFlagValue == common.DefaultOpniNamespace {
				if err := common.CreateDefaultNamespace(cmd.Context()); err != nil {
					return err
				}
			}

			opniCluster := buildOpniCluster(vars)
			if vars.edit {
				err := cliutil.EditObject(opniCluster, common.K8sClient.Scheme())
				if err != nil {
					if errors.Is(err, cliutil.ErrCanceled) {
						common.Log.Info("OpniCluster creation canceled.")
						return nil
					}
					return err
				}
			}

			err = common.K8sClient.Create(cmd.Context(), opniCluster)
			if ignoreAlreadyExists(err) != nil {
				return err
			}

			if !vars.noPretrainedModel {
				pretrainedModel, err := buildPretrainedModel(cmd, vars)
				if ignoreAlreadyExists(err) != nil {
					return err
				}
				err = common.K8sClient.Create(cmd.Context(), pretrainedModel)
				if ignoreAlreadyExists(err) != nil {
					return err
				}
			}

			if !vars.noLogAdapter {
				logAdapter := buildLogAdapter(vars)
				err = common.K8sClient.Create(cmd.Context(), logAdapter)
				if ignoreAlreadyExists(err) != nil {
					return err
				}
			}

			if !vars.noGpuPolicyAdapter && featureGate.Enabled(features.GPUOperator) {
				gpuPolicyAdapter := buildGpuPolicyAdapter(vars)
				err = common.K8sClient.Create(cmd.Context(), gpuPolicyAdapter)
				if ignoreAlreadyExists(err) != nil {
					return err
				}
			}

			if !vars.noNodeFeatureDiscovery && featureGate.Enabled(features.NodeFeatureDiscoveryOperator) {
				nodeFeatureDiscovery := buildNodeFeatureDiscovery(vars)
				err = common.K8sClient.Create(cmd.Context(), nodeFeatureDiscovery)
				if ignoreAlreadyExists(err) != nil {
					return err
				}
			}

			waitCtx, ca := context.WithTimeout(cmd.Context(), common.TimeoutFlagValue)
			defer ca()
			return cliutil.WaitAndDisplayStatus(waitCtx, common.K8sClient, opniCluster)
		},
	}

	createClusterCmd.Flags().StringVar(&vars.name, "name", "", "resource name")
	createClusterCmd.Flags().StringVar(&vars.version, "version", "latest", "opni version to install")
	createClusterCmd.Flags().BoolVar(&vars.edit, "edit", false, "edit resource before creating it")
	createClusterCmd.Flags().BoolVar(&vars.noPretrainedModel, "no-pretrained-model", false, "do not deploy a PretrainedModel resource")
	createClusterCmd.Flags().BoolVar(&vars.noLogAdapter, "no-log-adapter", false, "do not deploy a LogAdapter resource")
	createClusterCmd.Flags().BoolVar(&vars.noGpuPolicyAdapter, "no-gpu-policy-adapter", false, "do not deploy a GpuPolicyAdapter resource")
	createClusterCmd.Flags().BoolVar(&vars.noNodeFeatureDiscovery, "no-node-feature-discovery", false, "do not deploy a NodeFeatureDiscovery resource")
	createClusterCmd.Flags().BoolVar(&vars.interactive, "interactive", isatty.IsTerminal(os.Stdout.Fd()), "use interactive prompts (disabling this will use only defaults)")
	createClusterCmd.Flags().StringVar(&vars.provider, "provider", "", "kubernetes provider name (leave unset to auto-detect)")
	createClusterCmd.Flags().StringVar(&vars.controlPlaneModelSource, "control-plane-model-source", "http", "control plane model source (http|image)")
	createClusterCmd.Flags().StringVar(&vars.controlPlaneModelURL, "control-plane-model-url", "", "optional alternative control plane model URL")
	createClusterCmd.Flags().StringVar(&vars.controlPlaneModelImage, "control-plane-model-image", "", "control plane model image (if source is \"image\")")

	createClusterCmd.MarkFlagRequired("name")

	return createClusterCmd
}
