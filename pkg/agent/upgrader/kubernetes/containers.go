package kubernetes

import (
	"context"
	"fmt"
	"strings"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"golang.org/x/exp/slices"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (k *KubernetesAgentUpgrader) getAgentDeployment(ctx context.Context) (*appsv1.Deployment, error) {
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

func (k *KubernetesAgentUpgrader) getAgentContainer(containers []corev1.Container) *corev1.Container {
	for _, container := range containers {
		if slices.Contains(container.Args, "agentv2") {
			return &container
		}
	}
	return nil
}

func (k *KubernetesAgentUpgrader) getControllerContainer(containers []corev1.Container) *corev1.Container {
	for _, container := range containers {
		if slices.Contains(container.Args, "client") {
			return &container
		}
	}
	return nil
}

func (k *KubernetesAgentUpgrader) updateDeploymentImages(
	ctx context.Context,
	deploy *appsv1.Deployment,
	entries []*controlv1.UpdateManifestEntry,
) error {
	containers := deploy.Spec.Template.Spec.Containers

	agentImage := k.buildAgentImage(entries)
	controllerImage := k.buildClientImage(entries)

	if agentImage == "" {
		agentImage = controllerImage
	}

	agent := k.getAgentContainer(containers)
	if agent != nil && agentImage != "" {
		agent.Image = agentImage
		containers = replaceContainer(containers, agent)
	}

	controller := k.getControllerContainer(containers)
	if controller != nil && controllerImage != "" {
		controller.Image = controllerImage
		containers = replaceContainer(containers, controller)
	}

	deploy.Spec.Template.Spec.Containers = containers

	return nil
}

func (k *KubernetesAgentUpgrader) buildAgentImage(entries []*controlv1.UpdateManifestEntry) string {
	agentEntry := getEntry(string(agentPackageAgent), entries)
	if agentEntry == nil {
		return ""
	}
	path := strings.TrimPrefix(agentEntry.GetPath(), "oci://")
	path = k.maybeOverrideRepo(path)
	digest := agentEntry.GetDigest()
	if strings.HasPrefix(digest, "sha256:") {
		return fmt.Sprintf("%s@%s", path, digest)
	}
	return fmt.Sprintf("%s:%s", path, digest)
}

func (k *KubernetesAgentUpgrader) buildClientImage(entries []*controlv1.UpdateManifestEntry) string {
	agentEntry := getEntry(string(agentPackageClient), entries)
	if agentEntry == nil {
		return ""
	}
	path := strings.TrimPrefix(agentEntry.GetPath(), "oci://")
	path = k.maybeOverrideRepo(path)
	digest := agentEntry.GetDigest()
	if strings.HasPrefix(digest, "sha256:") {
		return fmt.Sprintf("%s@%s", path, digest)
	}
	return fmt.Sprintf("%s:%s", path, digest)
}

func (k *KubernetesAgentUpgrader) maybeOverrideRepo(path string) string {
	if k.repoOverride != nil {
		splitPath := strings.Split(path, "/")
		if strings.Contains(splitPath[0], ".") {
			path = strings.Replace(path, splitPath[0], *k.repoOverride, 1)
		} else {
			path = fmt.Sprintf("%s/%s", *k.repoOverride, path)
		}
	}
	return path
}

func replaceContainer(containers []corev1.Container, container *corev1.Container) []corev1.Container {
	for i, c := range containers {
		if c.Name == container.Name {
			containers[i] = *container
		}
	}
	return containers
}

func getEntry(packageName string, entries []*controlv1.UpdateManifestEntry) *controlv1.UpdateManifestEntry {
	for _, entry := range entries {
		if entry.GetPackage() == packageName {
			if strings.HasPrefix(entry.GetPath(), "oci://") {
				return entry
			}
		}
	}
	return nil
}
