package cortex

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (r *Reconciler) highlyAvailableStatefulSets() []*appsv1.StatefulSet {
	compactor := r.buildCortexStatefulSet("compactor",
		WithOverrides(r.spec.Cortex.Workloads.Compactor),
	)
	storeGateway := r.buildCortexStatefulSet("store-gateway",
		ServiceName("cortex-store-gateway-headless"),
	)
	ingester := r.buildCortexStatefulSet("ingester",
		Lifecycle(&corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/ingester/shutdown",
					Port: intstr.FromString("http-metrics"),
				},
			},
		}),
		WithOverrides(r.spec.Cortex.Workloads.Ingester),
	)
	alertmanager := r.buildCortexStatefulSet("alertmanager",
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
	querier := r.buildCortexStatefulSet("querier",
		Ports(HTTP),
		NoPersistentStorage(),
		WithOverrides(r.spec.Cortex.Workloads.Querier),
	)
	return []*appsv1.StatefulSet{
		alertmanager,
		ingester,
		compactor,
		storeGateway,
		querier,
	}
}

func (r *Reconciler) allInOneStatefulSets() []*appsv1.StatefulSet {
	all := r.buildCortexStatefulSet("all",
		WithOverrides(r.spec.Cortex.Workloads.AllInOne),
		Replicas(1), // Force replicas to 1 for all-in-one mode
	)
	return []*appsv1.StatefulSet{
		all,
	}
}
