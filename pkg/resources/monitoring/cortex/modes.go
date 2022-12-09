package cortex

import (
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/resources"
)

func (r *Reconciler) deployments() []resources.Resource {
	aio := r.allInOneDeployments()
	ha := r.highlyAvailableDeployments()
	list := []resources.Resource{}
	for _, res := range aio {
		list = append(list, resources.PresentIff(
			r.mc.Spec.Cortex.DeploymentMode == corev1beta1.DeploymentModeAllInOne, res))
	}
	for _, res := range ha {
		list = append(list, resources.PresentIff(
			r.mc.Spec.Cortex.DeploymentMode == corev1beta1.DeploymentModeHighlyAvailable, res))
	}
	return list
}

func (r *Reconciler) statefulSets() []resources.Resource {
	aio := r.allInOneStatefulSets()
	ha := r.highlyAvailableStatefulSets()
	list := []resources.Resource{}
	for _, res := range aio {
		list = append(list, resources.PresentIff(
			r.mc.Spec.Cortex.DeploymentMode == corev1beta1.DeploymentModeAllInOne, res))
	}
	for _, res := range ha {
		list = append(list, resources.PresentIff(
			r.mc.Spec.Cortex.DeploymentMode == corev1beta1.DeploymentModeHighlyAvailable, res))
	}
	return list
}
