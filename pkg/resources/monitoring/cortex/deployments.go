package cortex

import (
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
)

func (r *Reconciler) highlyAvailableDeployments() []resources.Resource {
	distributor := r.buildCortexDeployment("distributor",
		Replicas(1),
		WithOverrides(r.spec.Cortex.Workloads.Distributor),
	)
	queryFrontend := r.buildCortexDeployment("query-frontend",
		Replicas(1),
		Ports(HTTP, GRPC),
		WithOverrides(r.spec.Cortex.Workloads.QueryFrontend),
	)
	purger := r.buildCortexDeployment("purger",
		Replicas(1),
		Ports(HTTP),
		WithOverrides(r.spec.Cortex.Workloads.Purger),
	)
	ruler := r.buildCortexDeployment("ruler",
		Ports(HTTP, Gossip),
		ExtraVolumeMounts(corev1.VolumeMount{
			Name:      "tmp",
			MountPath: "/rules",
		}),
		ExtraVolumes(corev1.Volume{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}),
		WithOverrides(r.spec.Cortex.Workloads.Ruler),
	)
	return []resources.Resource{
		distributor,
		queryFrontend,
		ruler,
		purger,
	}
}

func (r *Reconciler) allInOneDeployments() []resources.Resource {
	return []resources.Resource{}
}
