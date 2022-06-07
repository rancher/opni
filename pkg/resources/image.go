package resources

import (
	"context"
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func FindManagerImage(ctx context.Context, c client.Client) (string, corev1.PullPolicy, error) {
	var pullPolicy corev1.PullPolicy

	podName, ok := os.LookupEnv("POD_NAME")
	if !ok {
		return "", "", fmt.Errorf("POD_NAME not set")
	}
	podNamespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		return "", "", fmt.Errorf("POD_NAMESPACE not set")
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(pod), pod); err != nil {
		return "", "", err
	}

	var imageID string
	for i, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name == "manager" {
			imageID = containerStatus.ImageID
			pullPolicy = pod.Spec.Containers[i].ImagePullPolicy
		}
	}
	if imageID == "" {
		return "", "", fmt.Errorf("manager container not found")
	}

	// depending on the container runtime, the prefix docker-pullable:// may
	// be prepended to the image ID.
	imageID = strings.TrimPrefix(imageID, "docker-pullable://")

	return imageID, pullPolicy, nil
}
