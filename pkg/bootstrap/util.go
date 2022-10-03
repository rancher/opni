package bootstrap

import (
	"context"
	"fmt"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"sigs.k8s.io/yaml"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Erases the bootstrap tokens from the agent-config secret.
// if restConfig is nil, InClusterConfig will be used.
func eraseBootstrapTokensFromConfig(
	ctx context.Context,
	restConfig *rest.Config,
	namespace string,
) error {
	if restConfig == nil {
		var err error
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return err
		}
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	secret, err := clientset.CoreV1().
		Secrets(namespace).
		Get(ctx, "agent-config", metav1.GetOptions{})
	if err != nil {
		return err
	}
	data := secret.Data["config.yaml"]
	agentConfig := v1beta1.AgentConfig{}
	err = yaml.Unmarshal(data, &agentConfig)
	if err != nil {
		return err
	}
	if agentConfig.Spec.Bootstrap == nil {
		return nil
	}
	agentConfig.Spec.Bootstrap = nil
	data, err = yaml.Marshal(agentConfig)
	if err != nil {
		return err
	}
	secret.Data["config.yaml"] = data
	_, err = clientset.CoreV1().
		Secrets(namespace).
		Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update agent config secret: %w", err)
	}
	return nil
}
