package cortex

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func (r *Reconciler) highlyAvailableDeployments() []*appsv1.Deployment {
	distributor := r.buildCortexDeployment("distributor",
		Replicas(1),
		WithOverrides(r.mc.Spec.Cortex.Workloads.Distributor),
	)
	queryFrontend := r.buildCortexDeployment("query-frontend",
		Replicas(1),
		Ports(HTTP, GRPC),
		WithOverrides(r.mc.Spec.Cortex.Workloads.QueryFrontend),
	)
	purger := r.buildCortexDeployment("purger",
		Replicas(1),
		Ports(HTTP),
		WithOverrides(r.mc.Spec.Cortex.Workloads.Purger),
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
		WithOverrides(r.mc.Spec.Cortex.Workloads.Ruler),
	)
	return []*appsv1.Deployment{
		distributor,
		queryFrontend,
		ruler,
		purger,
	}
}

func (r *Reconciler) allInOneDeployments() []*appsv1.Deployment {
	return []*appsv1.Deployment{}
}
