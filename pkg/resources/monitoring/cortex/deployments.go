package cortex

import (
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (r *Reconciler) deployments() []resources.Resource {
	alertmanager := r.buildCortexDeployment("alertmanager",
		ExtraVolumes(corev1.Volume{
			Name: "fallback-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "alertmanager-fallback-config",
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "fallback.yaml",
							Path: "fallback.yaml",
						},
					},
				},
			},
		}),
		ExtraVolumeMounts(corev1.VolumeMount{
			Name:      "fallback-config",
			MountPath: "/etc/alertmanager/fallback.yaml",
			SubPath:   "fallback.yaml",
		}),
	)
	distributor := r.buildCortexDeployment("distributor",
		Replicas(1),
	)
	ingester := r.buildCortexDeployment("ingester",
		Lifecycle(&corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/ingester/shutdown",
					Port: intstr.FromString("http-metrics"),
				},
			},
		}),
	)
	querier := r.buildCortexDeployment("querier",
		Ports(HTTP),
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
		alertmanager,
		distributor,
		ingester,
		querier,
		queryFrontend,
		ruler,
	}
}
