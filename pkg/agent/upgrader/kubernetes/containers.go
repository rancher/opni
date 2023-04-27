package kubernetes

import (
	"context"

	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/emptypb"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (k *kubernetesAgentUpgrader) getAgentDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{}
	err := k.k8sClient.Get(ctx, client.ObjectKey{
		Name:      "opni-agent",
		Namespace: k.namespace,
	}, deployment)
	if err != nil {
		return nil, err
	}
	return deployment, nil
}

func (k *kubernetesAgentUpgrader) getAgentContainer(containers []corev1.Container) *corev1.Container {
	for _, container := range containers {
		if slices.Contains(container.Args, "agentv2") {
			return &container
		}
	}
	return nil
}

func (k *kubernetesAgentUpgrader) getControllerContainer(containers []corev1.Container) *corev1.Container {
	for _, container := range containers {
		if slices.Contains(container.Args, "client") {
			return &container
		}
	}
	return nil
}

func (k *kubernetesAgentUpgrader) updateDeploymentImages(ctx context.Context, deploy *appsv1.Deployment) error {
	if k.imageClient == nil {
		return ErrGatewayClientNotSet
	}

	images, err := k.imageClient.GetAgentImages(ctx, &emptypb.Empty{})
	if err != nil {
		return err
	}

	containers := deploy.Spec.Template.Spec.Containers

	agent := k.getAgentContainer(containers)
	if agent != nil {
		agent.Image = images.AgentImage
		containers = replaceContainer(containers, agent)
	}

	controller := k.getControllerContainer(containers)
	if controller != nil {
		controller.Image = images.ControllerImage
		containers = replaceContainer(containers, controller)
	}

	deploy.Spec.Template.Spec.Containers = containers

	return nil
}

func replaceContainer(containers []corev1.Container, container *corev1.Container) []corev1.Container {
	for i, c := range containers {
		if c.Name == container.Name {
			containers[i] = *container
		}
	}
	return containers
}
