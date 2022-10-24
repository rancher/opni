package cortex

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		NoLivenessProbe(),
		NoStartupProbe(),
		TerminationGracePeriodSeconds(600),
		VolumeClaimTemplates(corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "data",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("2Gi"),
					},
				},
			},
		}),
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
		Replicas(1), // Force replicas to 1 for all-in-one mode
		NoLivenessProbe(),
		NoStartupProbe(),
		TerminationGracePeriodSeconds(600),
		VolumeClaimTemplates(corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "data",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("64Gi"), // TODO: Make this configurable
					},
				},
			},
		}),
	)
	return []*appsv1.StatefulSet{
		all,
	}
}
