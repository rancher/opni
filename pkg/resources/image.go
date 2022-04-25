package resources

import (
	"context"
	"fmt"
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func FindManagerImage(ctx context.Context, c client.Client) (string, corev1.PullPolicy, error) {
	var image string
	var pullPolicy corev1.PullPolicy

	deploymentNamespace := os.Getenv("OPNI_SYSTEM_NAMESPACE")
	if deploymentNamespace == "" {
		panic("missing downward API env vars")
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-controller-manager",
			Namespace: deploymentNamespace,
		},
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(deployment), deployment); err != nil {
		return "", "", err
	}
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == "manager" {
			image = container.Image
			pullPolicy = container.ImagePullPolicy
		}
	}

	if image == "" {
		panic(fmt.Sprintf("manager container not found in deployment %s/opni-controller-manager", deploymentNamespace))
	}

	return image, pullPolicy, nil
}
