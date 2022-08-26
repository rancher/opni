package cortex

import (
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/resources"
)

func (r *Reconciler) deployments() []resources.Resource {
	switch r.spec.Cortex.DeploymentMode {
	case corev1beta1.DeploymentModeAllInOne:
		return r.allInOneDeployments()
	case corev1beta1.DeploymentModeHighlyAvailable:
		return r.highlyAvailableDeployments()
	}
	r.logger.With(
		"mode", r.spec.Cortex.DeploymentMode,
	).Fatal("unknown deployment mode")
	return nil
}

func (r *Reconciler) statefulSets() []resources.Resource {
	switch r.spec.Cortex.DeploymentMode {
	case corev1beta1.DeploymentModeAllInOne:
		return r.allInOneStatefulSets()
	case corev1beta1.DeploymentModeHighlyAvailable:
		return r.highlyAvailableStatefulSets()
	}
	r.logger.With(
		"mode", r.spec.Cortex.DeploymentMode,
	).Fatal("unknown deployment mode")
	return nil
}
