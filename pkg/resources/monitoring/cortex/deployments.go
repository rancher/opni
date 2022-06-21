package cortex

import (
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
)

func (r *Reconciler) deployments() []resources.Resource {
	distributor := r.buildCortexDeployment("distributor",
		Replicas(1),
	)
	queryFrontend := r.buildCortexDeployment("query-frontend",
		Replicas(1),
		Ports(HTTP, GRPC),
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
	)
	return []resources.Resource{
		distributor,
		queryFrontend,
		ruler,
	}
}
