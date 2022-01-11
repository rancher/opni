package commands

import (
	"context"
	"fmt"
	"log"

	"github.com/kralicky/opni-monitoring/pkg/config/meta"
	"github.com/kralicky/opni-monitoring/pkg/config/v1beta1"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubectl/pkg/polymorphichelpers"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/yaml"
)

func BuildBootstrapCmd() *cobra.Command {
	var namespace, kubeconfig, gatewayAddress, token string
	var pins []string
	bootstrapCmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap an agent",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()

			rules := clientcmd.NewDefaultClientConfigLoadingRules()
			rules.ExplicitPath = kubeconfig
			apiConfig, err := rules.Load()
			if err != nil {
				log.Println(fmt.Errorf("failed to load kubeconfig: %w", err))
				return
			}
			restConfig, err := clientcmd.NewDefaultClientConfig(
				*apiConfig, &clientcmd.ConfigOverrides{}).ClientConfig()
			if err != nil {
				log.Println(fmt.Errorf("failed to load kubeconfig: %w", err))
				return
			}
			clientset := kubernetes.NewForConfigOrDie(restConfig)

			log.Println("Checking for pending agents...")

			dep, err := clientset.AppsV1().
				Deployments(namespace).
				Get(ctx, "opni-monitoring-agent", metav1.GetOptions{})
			if err != nil {
				log.Println(fmt.Errorf("failed to look up agent deployment: %w", err))
				return
			}

			// If any replicas are available (not unavailable), the agent has already
			// been bootstrapped.
			if dep.Status.UnavailableReplicas < pointer.Int32Deref(dep.Spec.Replicas, 1) {
				log.Println("Agent has already been bootstrapped.")
				return
			}

			// If the agent config secret exists, the agent has already been
			// bootstrapped
			_, err = clientset.CoreV1().
				Secrets(namespace).
				Get(ctx, "agent-config", metav1.GetOptions{})
			if err == nil {
				log.Println("Agent has already been bootstrapped")
				return
			}
			if !k8serrors.IsNotFound(err) {
				log.Println(fmt.Errorf("failed to look up agent config secret: %v", err))
				return
			}
			log.Println("Bootstrapping agent...")

			agentConfig := v1beta1.AgentConfig{
				TypeMeta: meta.TypeMeta{
					APIVersion: "v1beta1",
					Kind:       "AgentConfig",
				},
				Spec: v1beta1.AgentConfigSpec{
					ListenAddress:  ":8080",
					GatewayAddress: gatewayAddress,
					IdentityProvider: v1beta1.IdentityProviderSpec{
						Type: v1beta1.IdentityProviderKubernetes,
					},
					Storage: v1beta1.StorageSpec{
						Type: v1beta1.StorageTypeSecret,
					},
					Bootstrap: v1beta1.BootstrapSpec{
						Token: token,
						Pins:  pins,
					},
				},
			}

			configData, err := yaml.Marshal(agentConfig)
			if err != nil {
				log.Println(fmt.Errorf("failed to marshal agent config: %w", err))
				return
			}

			secret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "agent-config",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"config.yaml": configData,
				},
			}
			_, err = clientset.CoreV1().
				Secrets(namespace).
				Create(ctx, &secret, metav1.CreateOptions{})
			if err != nil {
				log.Println(fmt.Errorf("failed to create agent config secret: %w", err))
				return
			}

			// Check if the deployment needs to be restarted
			for _, cond := range dep.Status.Conditions {
				if cond.Type == appsv1.DeploymentProgressing &&
					cond.Status == corev1.ConditionFalse &&
					cond.Reason == "ProgressDeadlineExceeded" {
					log.Println("Restarting agent deployment...")
					if err := doRolloutRestart(clientset, dep); err != nil {
						log.Println(fmt.Errorf("failed to restart agent deployment: %w", err))
						return
					}
				}
			}

			log.Println("Done.")
		},
	}

	bootstrapCmd.Flags().StringVarP(&gatewayAddress, "address", "a", "", "Gateway address")
	bootstrapCmd.Flags().StringVarP(&token, "token", "t", "", "Token to use for bootstrapping")
	bootstrapCmd.Flags().StringSliceVar(&pins, "pin", []string{}, "Gateway server public key to pin (repeatable)")
	bootstrapCmd.Flags().StringVarP(&namespace, "namespace", "n", "opni-monitoring-agent", "Namespace where the agent is installed")
	bootstrapCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (optional)")

	bootstrapCmd.MarkFlagRequired("token")
	bootstrapCmd.MarkFlagRequired("address")
	bootstrapCmd.MarkFlagRequired("pin")

	return bootstrapCmd
}

func doRolloutRestart(clientset *kubernetes.Clientset, dep *appsv1.Deployment) error {
	patch, err := polymorphichelpers.ObjectRestarterFn(dep)
	if err != nil {
		return err
	}
	_, err = clientset.AppsV1().
		Deployments(dep.Namespace).
		Patch(context.Background(),
			dep.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{
				FieldManager: "kubectl-rollout",
			},
		)
	return err
}
