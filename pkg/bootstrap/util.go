package bootstrap

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/kralicky/opni-monitoring/pkg/config/v1beta1"
	"sigs.k8s.io/yaml"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func EraseBootstrapTokensFromConfig() error {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return err
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	ctx := context.Background()
	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		log.Fatal("POD_NAMESPACE environment variable not set")
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
	agentConfig.Spec.Bootstrap = v1beta1.BootstrapSpec{}
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
